package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/rs/cors"
	"golang.org/x/crypto/acme/autocert"
)

type ShoppingList struct {
	ID    int      `json:"id"`
	Name  string   `json:"name"`
	Items []string `json:"items"`
}

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

	corsMiddleware := cors.New(cors.Options{
		AllowedOrigins: []string{"http://localhost:3000"},
		AllowedMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodDelete,
			http.MethodPatch,
			http.MethodOptions,
		},
		AllowedHeaders: []string{
			"Content-Type",
			"Authorization",
		},
		MaxAge: 300,
	})

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/lists", adminRequired(handleCreateList))
	mux.HandleFunc("GET /v1/lists", authRequired(handleListLists))
	mux.HandleFunc("DELETE /v1/lists/{id}", adminRequired(handleDeleteList))
	mux.HandleFunc("PUT /v1/lists/{id}", adminRequired(handleUpdateList))
	mux.HandleFunc("PATCH /v1/lists/{id}", adminRequired(handlePatchList))
	mux.HandleFunc("GET /v1/lists/{id}", authRequired(handleGetList))
	mux.HandleFunc("POST /v1/lists/{id}/push", adminRequired(handleListPush))
	mux.HandleFunc("POST /login", handleLogin)

	corsHandler := corsMiddleware.Handler(mux)

	env := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))

	// Run local HTTP unless production is explicitly requested.
	if env == "" || env == "local" || env == "dev" || env == "development" {
		fmt.Println("listening on http://localhost:8888")
		err := http.ListenAndServe(":8888", corsHandler)
		if err != nil {
			log.Fatal(err)
		}
		return
	}

	certManager := autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist("infotrod.com"),
		Cache:      autocert.DirCache("certs"),
	}

	server := &http.Server{
		Addr:      ":https",
		Handler:   corsHandler,
		TLSConfig: certManager.TLSConfig(),
	}

	go func() {
		err := http.ListenAndServe(":http", certManager.HTTPHandler(nil))
		if err != nil {
			log.Fatal(err)
		}
	}()

	log.Println("listening on https")
	log.Fatal(server.ListenAndServeTLS("", ""))
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
	var item ListPushAction
	err := json.NewDecoder(r.Body).Decode(&item)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = repository.AddItemToShoppingList(id, &item)
	if err != nil {
		http.Error(w, "List not found", http.StatusNotFound)
		return
	}
	list, err := repository.GetShoppingList(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = json.NewEncoder(w).Encode(list)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
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
