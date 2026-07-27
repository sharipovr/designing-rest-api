package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type ShoppingList struct {
	ID    int      `json:"id"`
	Name  string   `json:"name"`
	Items []string `json:"items"`
}

var allData []ShoppingList

type ShoppingListPatch struct {
	Name  *string  `json:"name"`
	Items []string `json:"items"`
}

type ListPushAction struct {
	Item string `json:"item"`
}

type User struct {
	Role     string
	Username string
	Password string
}

type Session struct {
	Token    string
	Expires  time.Time
	Username string
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

var sessions = map[string]*Session{}
var allUsers = map[string]*User{
	"admin": {"admin", "admin", "password"},
	"user":  {"user", "user", "password"},
}

var repository *Repository

func main() {
	var err error
	repository, err = NewRepository("./database.db")
	if err != nil {
		fmt.Println("Unable to open the database:", err.Error())
		os.Exit(1)
	}
	if err := repository.Init(); err != nil {
		fmt.Println("Unable to initialize the database:", err.Error())
		os.Exit(1)
	}

	http.HandleFunc("POST /v1/lists", adminRequired(handleCreateList))
	http.HandleFunc("GET /v1/lists", authRequired(handleListLists))
	http.HandleFunc("DELETE /v1/lists/{id}", adminRequired(handleDeleteList))
	http.HandleFunc("PUT /v1/lists/{id}", adminRequired(handleUpdateList))
	http.HandleFunc("PATCH /v1/lists/{id}", adminRequired(handlePatchList))
	http.HandleFunc("GET /v1/lists/{id}", authRequired(handleGetList))
	http.HandleFunc("POST /v1/lists/{id}/push", adminRequired(handleListPush))
	http.HandleFunc("POST /login", handleLogin)
	fmt.Println("listening on port :8888")
	http.ListenAndServe(":8888", nil)
}

func handleCreateList(w http.ResponseWriter, r *http.Request) {
	var list ShoppingList
	err := json.NewDecoder(r.Body).Decode(&list)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	list.ID = rand.Int()
	err = repository.CreateShoppingList(&list)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	err = json.NewEncoder(w).Encode(list)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func handleListLists(w http.ResponseWriter, r *http.Request) {
	lists, err := repository.GetShoppingLists()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data, err := json.Marshal(lists)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, err = w.Write(data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func handleDeleteList(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := repository.DeleteShoppingList(id)
	if err != nil {
		http.Error(w, "List not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleUpdateList(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var updatedList ShoppingList
	err := json.NewDecoder(r.Body).Decode(&updatedList)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err = repository.UpdateShoppingList(id, &updatedList)
	if err != nil {
		http.Error(w, "List not found", http.StatusNotFound)
		return
	}

	if err := json.NewEncoder(w).Encode(updatedList); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func handlePatchList(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var patch ShoppingListPatch
	err := json.NewDecoder(r.Body).Decode(&patch)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = repository.PatchShoppingList(id, &patch)
	if err != nil {
		http.Error(w, "List not found", http.StatusNotFound)
		return
	}

	list, err := repository.GetShoppingList(id)
	if err != nil {
		http.Error(w, "List not found", http.StatusNotFound)
		return
	}

	err = json.NewEncoder(w).Encode(list)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func handleGetList(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	list, err := repository.GetShoppingList(id)
	if err != nil {
		http.Error(w, "List not found", http.StatusNotFound)
		return
	}

	data, err := json.Marshal(list)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, err = w.Write(data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func handleListPush(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	for i, list := range allData {
		if strconv.Itoa(list.ID) == id {
			var item ListPushAction
			err := json.NewDecoder(r.Body).Decode(&item)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			list.Items = append(list.Items, item.Item)
			allData[i] = list
			err = json.NewEncoder(w).Encode(list)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			return
		}
	}
	http.Error(w, "List not found", http.StatusNotFound)
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	var data LoginRequest
	err := json.NewDecoder(r.Body).Decode(&data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	user := allUsers[data.Username]
	if user != nil && user.Password == data.Password {
		session, err := repository.AddSession(user.Username)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode(map[string]string{"token": session.Token})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		return
	}
	w.WriteHeader(http.StatusUnauthorized)
}

func authRequired(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if !strings.HasPrefix(token, "Bearer ") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		token = token[7:]
		_, err := repository.GetSession(token)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func adminRequired(next http.HandlerFunc) http.HandlerFunc {
	return authRequired(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		token = token[7:]
		session, err := repository.GetSession(token)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		user := allUsers[session.Username]
		if user.Role != "admin" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	})
}
