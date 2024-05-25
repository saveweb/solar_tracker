import copy
from dataclasses import dataclass
import re
import json
import time
from typing import Any, Dict, Optional, Union

import requests


@dataclass
class ProjectMeta:
    identifier: str
    slug: str
    icon: str
    deadline: str
@dataclass
class ProjectStatus:
    public: bool
    paused: bool
@dataclass
class ProjectClient:
    version: str
    claim_task_delay: float
    """ 客户端在 claim_task 之后，多久之后才能再次 claim_task （软限制）"""
@dataclass
class ProjectMongodb:
    db_name: str
    item_collection: str
    queue_collection: str
    custom_doc_id_name: str
    """ TODO: 未来弃用 """
@dataclass
class Project:
    meta: ProjectMeta
    status: ProjectStatus
    client: ProjectClient
    mongodb: ProjectMongodb

    def __init__(self, meta: dict, status: dict, client: dict, mongodb: dict):
        self.meta = ProjectMeta(**meta)
        self.status = ProjectStatus(**status)
        self.client = ProjectClient(**client)
        self.mongodb = ProjectMongodb(**mongodb)


TRACKER_NODES = [
    "https://0.tracker.saveweb.org/",
    "https://1.tracker.saveweb.org/",
    "https://2.tracker.saveweb.org/",
    "http://3.tracker.saveweb.org/",
]

TEST_TRACKER_NODES = [
    "http://localhost:8080/",
]

class Tracker:
    API_BASE: str = TEST_TRACKER_NODES[0]
    API_VERSION = "v1"
    client_version: str
    project_id: str
    """ [^a-zA-Z0-9_\\-] """
    archivist: str
    """ [^a-zA-Z0-9_\\-] """
    session: requests.Session
    project_id: str
    __project: Project
    __project_last_fetched: float = 0

    __last_task_claimed_at: float = 0

    def _is_safe_string(self, s: str):
        r = re.compile(r"[^a-zA-Z0-9_\\-]")
        return not r.search(s)

    def __init__(self, project_id: str, client_version: str, archivist: str, session: requests.Session):
        assert self._is_safe_string(project_id) and self._is_safe_string(archivist), "[^a-zA-Z0-9_\\-]"
        self.project_id = project_id
        self.client_version = client_version
        self.archivist = archivist
        self.session = session

        self.select_best_tracker()

        assert self.project.client.version == self.client_version, "client_version mismatch, please upgrade your client."

        print(f"[tracker] Hello, {self.archivist}!")
        print(f"[tracker] Project: {self.project}")
    
    @property
    def project(self):
        """
        return: Project, deep copy.
        return __project cache if it's not expired (60s)
        """
        if time.time() - self.__project_last_fetched > 60:
            self.__project = self.fetch_project()
            self.__project_last_fetched = time.time()
        return copy.deepcopy(self.__project)
    
    def select_best_tracker(self):
        result = [] # [(node, ping)]
        print("[client->trackers] select_best_tracker...")
        for node in TRACKER_NODES:
            try:
                self.session.get(node + 'ping', timeout=0.05) # DNS preload, dirty hack
            except Exception:
                pass

        for node in TRACKER_NODES:
            print(f"[client->tracker({node})] ping...")
            start = time.time()
            try:
                r = self.session.get(node + 'ping', timeout=5)
                r.raise_for_status()
                print(f"[client<-tracker({node})] ping OK. {time.time() - start:.2f}s")
                result.append((node, time.time() - start))
            except Exception as e:
                print(f"[client->tracker({node}) ping failed. {type(e)}")
                result.append((node, float('inf')))
        result.sort(key=lambda x: x[1])
        self.API_BASE = result[0][0]
        print("===============================")
        print(f"tracker selected: {self.API_BASE}")
        print("===============================")

    def fetch_project(self):
        """
        return: Project, deep copy
        also refresh __project cache
        """
        assert isinstance(self.session, (requests.Session))
        print("[client->tracker] fetch_project...")
        r = self.session.post(self.API_BASE + self.API_VERSION + '/project/' + self.project_id)
        r.raise_for_status()
        proj = Project(**r.json())
        self.__project = copy.deepcopy(proj)
        self.__project_last_fetched = time.time()
        print("[client<-tracker] project fetched.")
        return proj

    def get_projects(self):
        assert isinstance(self.session, (requests.Session))
        print("[client->tracker] get_projects")
        r = self.session.post(self.API_BASE + self.API_VERSION + '/projects')
        r.raise_for_status()
        print("[client<-tracker] get_projects. OK")
        return [Project(**p) for p in r.json()]


    def claim_task(self, with_delay: bool = True):
        assert isinstance(self.session, (requests.Session))
        if with_delay:
            if sleep_need := self.project.client.claim_task_delay - (time.time() - self.__last_task_claimed_at):
                if sleep_need > 0:
                    print(f"[tracker] slow down you {sleep_need:.2f}s, Qos: {self.project.client.claim_task_delay}")
                    time.sleep(sleep_need)
                elif sleep_need < 0:
                    print(f"[tracker] you are {sleep_need:.2f}s late, Qos: {self.project.client.claim_task_delay}")

        print("[client->tracker] claim_task")
        start = time.time()
        r = self.session.post(f'{self.API_BASE}{self.API_VERSION}/project/{self.project_id}/{self.client_version}/{self.archivist}/claim_task')
        self.__last_task_claimed_at = time.time()
        if r.status_code == 404:
            print(f'[client<-tracker] claim_task. (time cost: {time.time() - start:.2f}s):', r.text)
            return None # No tasks available
        if r.status_code == 200:
            r_json = r.json()
            print(f'[client<-tracker] claim_task. OK (time cost: {time.time() - start:.2f}s):', r_json)
            return r_json
        raise Exception(r.status_code, r.text)
    
    def update_task(self, task_id: Union[str, int], status: str)->Dict[str, Any]:
        """
        task_id: 必须明确传入的是 int 还是 str
        status: 任务状态
        """
        assert isinstance(self.session, (requests.Session))
        assert isinstance(task_id, (str, int))
        assert isinstance(status, str)
        
        print(f"[client->tracker] update_task task({task_id}) to status({status})")
        r = self.session.post(
            f'{self.API_BASE}{self.API_VERSION}/project/{self.project_id}/{self.client_version}/{self.archivist}/update_task/{task_id}', 
            data={
                'status': status,
                'task_id_type': 'int' if isinstance(task_id, int) else 'str'
            })
        if r.status_code == 200:
            r_json = r.json()
            print(f'[client<-tracker] update_task task({task_id}). OK:', r_json)
            return r_json
        raise Exception(r.text)
    
    def insert_item(self, item_id: Union[str, int], item_status: Optional[Union[str, int]] = None, payload: Optional[dict] = None)->Dict[str, Any]:
        """
        item_id: 必须明确传入的是 int 还是 str
        item_status: item 状态，而不是 task 状态。可用于标记一些被删除、被隐藏的 item 之类的。就不用添加一堆错误响应到 payload 里了。 
        payload: 可以传入任意的可转为 ext-json 的对象 (包括 None)
        """
        assert isinstance(self.session, (requests.Session))
        # item_id_type
        if isinstance(item_id, int):
            item_id_type = 'int'
        elif isinstance(item_id, str):
            item_id_type = 'str'
        else:
            raise ValueError("item_id must be int or str")

        if item_status is None:
            status_type = "None"
        elif isinstance(item_status, int):
            status_type = "int"
        elif isinstance(item_status, str):
            status_type = "str"
        else:
            raise ValueError("status must be int, str or None")
        payload_json_str = json.dumps(payload, ensure_ascii=False, separators=(',', ':'))
        print(f"[client->tracker] insert_item item({item_id}), len(payload)={len(payload_json_str)}")
        r = self.session.post(
            f'{self.API_BASE}{self.API_VERSION}/project/{self.project_id}/{self.client_version}/{self.archivist}/insert_item/{item_id}',
            data={
                'item_id_type': item_id_type,
                'item_status': item_status,
                'item_status_type': status_type,
                'payload': payload_json_str
            })
        if r.status_code == 200:
            r_json = r.json()
            print(f'[client<-tracker] insert_item item({item_id}). OK:', r_json)
            return r_json
        raise Exception(r.text)
