package main

import (
	"net/http"
)

type ShoppingList struct {
	ID    int      `json:"id"`
	Name  string   `json:"name"`
	Items []string `json:"items"`
}

var allData []ShoppingList

func main() {
	http.ListenAndServe(":8888", nil)
}
