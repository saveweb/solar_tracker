package main

type Item struct {
	Item_id          string `json:"item_id" binding:"required"`
	Item_id_type     string `json:"item_id_type" binding:"required,oneof=str int"`
	Item_status      string `json:"item_status" binding:"required"`
	Item_status_type string `json:"item_status_type" binding:"required,oneof=None str int"`
	Payload          string `json:"payload" binding:"required"`
}
