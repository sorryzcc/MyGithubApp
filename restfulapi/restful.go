package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// User 是示例资源。
type User struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type userInput struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type userPatch struct {
	Name  *string `json:"name"`
	Email *string `json:"email"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// store 用内存保存数据，并加锁保证并发安全。
type store struct {
	mu     sync.RWMutex
	users  map[int]User
	nextID int
}

func newStore() *store {
	s := &store{users: make(map[int]User), nextID: 1}
	s.users[1] = User{ID: 1, Name: "Alice", Email: "alice@example.com"}
	s.users[2] = User{ID: 2, Name: "Bob", Email: "bob@example.com"}
	s.nextID = 3
	return s
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := decoder.Decode(v); err != nil {
		return err
	}
	return nil
}

// parseID 从路径变量解析资源 ID。
func parseID(r *http.Request) (int, error) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("无效的资源 ID")
	}
	return id, nil
}

// list 返回资源列表，支持 limit / offset 分页。
func (s *store) list(w http.ResponseWriter, r *http.Request) {
	limit, offset := 20, 0
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "limit 必须是非负整数")
			return
		}
		limit = n
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "offset 必须是非负整数")
			return
		}
		offset = n
	}

	s.mu.RLock()
	users := make([]User, 0, len(s.users))
	for _, u := range s.users {
		users = append(users, u)
	}
	s.mu.RUnlock()

	sort.Slice(users, func(i, j int) bool { return users[i].ID < users[j].ID })
	if offset >= len(users) {
		users = []User{}
	} else {
		users = users[offset:]
		if limit < len(users) {
			users = users[:limit]
		}
	}
	writeJSON(w, http.StatusOK, users)
}

func (s *store) get(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.mu.RLock()
	u, ok := s.users[id]
	s.mu.RUnlock()
	if !ok {
		writeError(w, http.StatusNotFound, "资源不存在")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *store) create(w http.ResponseWriter, r *http.Request) {
	var in userInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "请求体不是合法的 JSON")
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		writeError(w, http.StatusBadRequest, "name 不能为空")
		return
	}

	s.mu.Lock()
	u := User{ID: s.nextID, Name: in.Name, Email: in.Email}
	s.nextID++
	s.users[u.ID] = u
	s.mu.Unlock()

	writeJSON(w, http.StatusCreated, u)
}

func (s *store) replace(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var in userInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "请求体不是合法的 JSON")
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		writeError(w, http.StatusBadRequest, "name 不能为空")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[id]; !ok {
		writeError(w, http.StatusNotFound, "资源不存在")
		return
	}
	u := User{ID: id, Name: in.Name, Email: in.Email}
	s.users[id] = u
	writeJSON(w, http.StatusOK, u)
}

func (s *store) patch(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var in userPatch
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "请求体不是合法的 JSON")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		writeError(w, http.StatusNotFound, "资源不存在")
		return
	}
	if in.Name != nil {
		if strings.TrimSpace(*in.Name) == "" {
			writeError(w, http.StatusBadRequest, "name 不能为空")
			return
		}
		u.Name = *in.Name
	}
	if in.Email != nil {
		u.Email = *in.Email
	}
	s.users[id] = u
	writeJSON(w, http.StatusOK, u)
}

func (s *store) remove(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[id]; !ok {
		writeError(w, http.StatusNotFound, "资源不存在")
		return
	}
	delete(s.users, id)
	w.WriteHeader(http.StatusNoContent)
}

func newRouter(s *store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /users", s.list)
	mux.HandleFunc("POST /users", s.create)
	mux.HandleFunc("GET /users/{id}", s.get)
	mux.HandleFunc("PUT /users/{id}", s.replace)
	mux.HandleFunc("PATCH /users/{id}", s.patch)
	mux.HandleFunc("DELETE /users/{id}", s.remove)
	return mux
}

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           newRouter(newStore()),
		ReadHeaderTimeout: 5 * time.Second,
	}
	fmt.Printf("REST API 正在监听端口 %s\n", strings.TrimPrefix(addr, ":"))
	log.Fatal(srv.ListenAndServe())
}
