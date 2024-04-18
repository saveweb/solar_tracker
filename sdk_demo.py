import copy
from dataclasses import dataclass
import re
import json
import time
from typing import Optional, Union

import requests
import httpx


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



class Tracker:
    API_BASE = "http://localhost:8080/"
    API_VERSION = "v1"
    client_version: str
    project_id: str
    """ [^a-zA-Z0-9_\\-] """
    archivist: str
    """ [^a-zA-Z0-9_\\-] """
    session: requests.Session | httpx.Client | httpx.AsyncClient
    project_id: str
    __project: Project
    __project_last_fetched: float = 0


    def _is_safe_string(self, s: str):
        r = re.compile(r"[^a-zA-Z0-9_\\-]")
        return not r.search(s)

    def __init__(self, project_id: str, client_version: str, archivist: str, session: requests.Session | httpx.Client | httpx.AsyncClient):
        assert self._is_safe_string(project_id) and self._is_safe_string(archivist), "[^a-zA-Z0-9_\\-]"
        self.project_id = project_id
        self.client_version = client_version
        self.archivist = archivist
        self.session = session
    
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

    def fetch_project(self):
        """
        return: Project, deep copy
        also refresh __project cache
        """
        assert isinstance(self.session, (requests.Session, httpx.Client))
        r = self.session.post(self.API_BASE + self.API_VERSION + '/project/' + self.project_id)
        r.raise_for_status()
        proj = Project(**r.json())
        self.__project = copy.deepcopy(proj)
        return proj

    def get_projects(self):
        assert isinstance(self.session, (requests.Session, httpx.Client))
        r = self.session.post(self.API_BASE + self.API_VERSION + '/projects')
        r.raise_for_status()
        return [Project(**p) for p in r.json()]
    
    def validate_client_version(self):
        assert isinstance(self.session, (requests.Session, httpx.Client))
        if self.project.client.version != self.client_version:
            return False
        return True

    def claim_task(self):
        assert isinstance(self.session, (requests.Session, httpx.Client))
        r = self.session.post(f'{self.API_BASE}{self.API_VERSION}/project/{self.project_id}/{self.client_version}/{self.archivist}/claim_task')
        if r.status_code == 404:
            return None # No tasks available
        if r.status_code == 200:
            return r.json()
        raise Exception(r.text)
    
    def update_task(self, task_id: Union[str, int], status: str):
        """
        task_id: 必须明确传入的是 int 还是 str
        status: 任务状态
        """
        assert isinstance(self.session, (requests.Session, httpx.Client))
        assert isinstance(task_id, (str, int))
        assert isinstance(status, str)
        

        r = self.session.post(
            f'{self.API_BASE}{self.API_VERSION}/project/{self.project_id}/{self.client_version}/{self.archivist}/update_task/{task_id}', 
            data={
                'status': status,
                'task_id_type': 'int' if isinstance(task_id, int) else 'str'
            })
        if r.status_code == 200:
            return r.json()
        raise Exception(r.text)
    
    def insert_item(self, item_id: Union[str, int], item_status: Optional[Union[str, int]] = None, payload: Optional[dict] = None):
        """
        item_id: 必须明确传入的是 int 还是 str
        item_status: item 状态，而不是 task 状态。可用于标记一些被删除、被隐藏的 item 之类的。就不用添加一堆错误响应到 payload 里了。 
        payload: 可以传入任意的可转为 ext-json 的对象 (包括 None)
        """
        assert isinstance(self.session, (requests.Session, httpx.Client))
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
        r = self.session.post(
            f'{self.API_BASE}{self.API_VERSION}/project/{self.project_id}/{self.client_version}/{self.archivist}/insert_item/{item_id}',
            data={
                'item_id_type': item_id_type,
                'item_status': item_status,
                'item_status_type': status_type,
                'payload': json.dumps(payload, ensure_ascii=False, separators=(',', ':'))
            })
        if r.status_code == 200:
            return r.json()
        raise Exception(r.text)
if __name__ == '__main__':
    ss = requests.Session()
    proj_id = 'test_proj'
    tracker = Tracker(proj_id, '1.0','demo_tester', ss)
    print(tracker.get_projects())
    assert proj_id in [p.meta.identifier for p in tracker.get_projects()]
    print(tracker.project)
    task = tracker.claim_task()
    print(task)
    id_name = tracker.project.mongodb.custom_doc_id_name or 'id'
    assert task
    task_result = tracker.update_task(task_id=task[id_name], status='FAIL')
    print(task_result)
    payload = {'test_data': {'name': 'aaabbccc', 'id': 25}}
    item_result = tracker.insert_item(item_id=task[id_name], item_status=1, payload=payload)
    print(item_result)
    # 12,625,984