package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	StatusPending  = "pending"  // 审批中
	StatusApproved = "approved" // 已通过
	StatusReturned = "returned" // 已驳回，退回申请人

	StepOwner   = "需求owner"
	StepManager = "需求owner上级"

	StepPending  = "pending"
	StepApproved = "approved"
	StepRejected = "rejected"
)

var (
	ErrNotFound      = errors.New("审批单不存在")
	ErrConflict      = errors.New("当前状态不能执行该操作")
	ErrImmutable     = errors.New("审批中的申请不能修改，请等待审批结果")
	ErrInvalidStatus = errors.New("无效的审批状态")
)

// ApprovalStep 记录一个审批节点的处理结果。
type ApprovalStep struct {
	Name      string    `json:"name"`
	Approver  string    `json:"approver"`
	Status    string    `json:"status"`
	Remark    string    `json:"remark"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Approval 是一条审批单，固定经过「需求owner」→「需求owner上级」两个节点。
type Approval struct {
	ID               int            `json:"id"`
	Title            string         `json:"title"`
	Applicant        string         `json:"applicant"`
	Reason           string         `json:"reason"`
	Owner            string         `json:"owner"`
	OwnerManager     string         `json:"owner_manager"`
	Status           string         `json:"status"`
	CurrentStep      int            `json:"current_step"` // 0=需求owner，1=需求owner上级
	Steps            []ApprovalStep `json:"steps"`
	LastRejectReason string         `json:"last_reject_reason"` // 最近一次驳回意见，供申请人查看
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

type approvalInput struct {
	Title        string `json:"title"`
	Applicant    string `json:"applicant"`
	Reason       string `json:"reason"`
	Owner        string `json:"owner"`
	OwnerManager string `json:"owner_manager"`
}

type approvalPatch struct {
	Title        *string `json:"title"`
	Applicant    *string `json:"applicant"`
	Reason       *string `json:"reason"`
	Owner        *string `json:"owner"`
	OwnerManager *string `json:"owner_manager"`
}

type decisionInput struct {
	Remark string `json:"remark"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type store struct {
	mu        sync.RWMutex
	approvals map[int]Approval
	nextID    int
}

func newStep(name, approver string) ApprovalStep {
	return ApprovalStep{Name: name, Approver: approver, Status: StepPending}
}

func newSteps(owner, manager string) []ApprovalStep {
	return []ApprovalStep{newStep(StepOwner, owner), newStep(StepManager, manager)}
}

func newStore() *store {
	now := time.Now()
	s := &store{approvals: make(map[int]Approval), nextID: 1}
	s.approvals[1] = Approval{
		ID: 1, Title: "采购办公电脑", Applicant: "张三", Reason: "团队新成员设备更新",
		Owner: "李四", OwnerManager: "王五", Status: StatusPending, CurrentStep: 0,
		Steps: newSteps("李四", "王五"), CreatedAt: now.Add(-3 * time.Hour), UpdatedAt: now.Add(-3 * time.Hour),
	}
	s.approvals[2] = Approval{
		ID: 2, Title: "出差申请 - 上海", Applicant: "赵六", Reason: "客户现场拜访",
		Owner: "李四", OwnerManager: "王五", Status: StatusApproved, CurrentStep: 2,
		Steps: []ApprovalStep{
			{Name: StepOwner, Approver: "李四", Status: StepApproved, Remark: "同意", UpdatedAt: now.Add(-24 * time.Hour)},
			{Name: StepManager, Approver: "王五", Status: StepApproved, Remark: "按标准执行", UpdatedAt: now.Add(-20 * time.Hour)},
		},
		CreatedAt: now.Add(-26 * time.Hour), UpdatedAt: now.Add(-20 * time.Hour),
	}
	s.approvals[3] = Approval{
		ID: 3, Title: "团建费用报销", Applicant: "孙七", Reason: "部门季度团建",
		Owner: "李四", OwnerManager: "王五", Status: StatusReturned, CurrentStep: 0,
		Steps: []ApprovalStep{
			{Name: StepOwner, Approver: "李四", Status: StepRejected, Remark: "预算超支，请调整金额", UpdatedAt: now.Add(-44 * time.Hour)},
			newStep(StepManager, "王五"),
		},
		LastRejectReason: "预算超支，请调整金额",
		CreatedAt:        now.Add(-50 * time.Hour), UpdatedAt: now.Add(-44 * time.Hour),
	}
	s.nextID = 4
	return s
}

func validStatus(s string) bool {
	return s == StatusPending || s == StatusApproved || s == StatusReturned
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

func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrConflict), errors.Is(err, ErrImmutable), errors.Is(err, ErrInvalidStatus):
		writeError(w, http.StatusConflict, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "内部错误")
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	return json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v)
}

func decodeOptionalJSON(w http.ResponseWriter, r *http.Request, v any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := decoder.Decode(v); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func parseID(r *http.Request) (int, error) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("无效的审批单 ID")
	}
	return id, nil
}

func parsePagination(r *http.Request) (limit, offset int, err error) {
	limit, offset = 20, 0
	if v := r.URL.Query().Get("limit"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil || n < 0 {
			return 0, 0, fmt.Errorf("limit 必须是非负整数")
		}
		limit = n
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil || n < 0 {
			return 0, 0, fmt.Errorf("offset 必须是非负整数")
		}
		offset = n
	}
	return limit, offset, nil
}

func (s *store) list(w http.ResponseWriter, r *http.Request) {
	limit, offset, err := parsePagination(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	status := r.URL.Query().Get("status")
	if status != "" && !validStatus(status) {
		writeError(w, http.StatusBadRequest, ErrInvalidStatus.Error())
		return
	}

	s.mu.RLock()
	items := make([]Approval, 0, len(s.approvals))
	for _, a := range s.approvals {
		if status == "" || a.Status == status {
			items = append(items, a)
		}
	}
	s.mu.RUnlock()

	sort.Slice(items, func(i, j int) bool { return items[i].ID > items[j].ID })
	if offset >= len(items) {
		items = []Approval{}
	} else {
		items = items[offset:]
		if limit < len(items) {
			items = items[:limit]
		}
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *store) get(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.mu.RLock()
	a, ok := s.approvals[id]
	s.mu.RUnlock()
	if !ok {
		writeStoreError(w, ErrNotFound)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (s *store) create(w http.ResponseWriter, r *http.Request) {
	var in approvalInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "请求体不是合法的 JSON")
		return
	}
	for field, value := range map[string]string{
		"title": in.Title, "applicant": in.Applicant, "owner": in.Owner, "owner_manager": in.OwnerManager,
	} {
		if strings.TrimSpace(value) == "" {
			writeError(w, http.StatusBadRequest, field+" 不能为空")
			return
		}
	}

	s.mu.Lock()
	now := time.Now()
	a := Approval{
		ID: s.nextID, Title: in.Title, Applicant: in.Applicant, Reason: in.Reason,
		Owner: in.Owner, OwnerManager: in.OwnerManager,
		Status: StatusPending, CurrentStep: 0,
		Steps: newSteps(in.Owner, in.OwnerManager), CreatedAt: now, UpdatedAt: now,
	}
	s.nextID++
	s.approvals[a.ID] = a
	s.mu.Unlock()

	writeJSON(w, http.StatusCreated, a)
}

// patch 编辑申请。审批中不可改；被驳回退回后可改，改完再 resubmit。
func (s *store) patch(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var in approvalPatch
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "请求体不是合法的 JSON")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.approvals[id]
	if !ok {
		writeStoreError(w, ErrNotFound)
		return
	}
	if a.Status == StatusPending {
		writeStoreError(w, ErrImmutable)
		return
	}
	if a.Status == StatusApproved {
		writeStoreError(w, ErrConflict)
		return
	}

	required := []struct {
		field string
		value *string
	}{
		{"title", in.Title}, {"applicant", in.Applicant},
		{"owner", in.Owner}, {"owner_manager", in.OwnerManager},
	}
	for _, item := range required {
		if item.value != nil && strings.TrimSpace(*item.value) == "" {
			writeError(w, http.StatusBadRequest, item.field+" 不能为空")
			return
		}
	}
	if in.Title != nil {
		a.Title = *in.Title
	}
	if in.Applicant != nil {
		a.Applicant = *in.Applicant
	}
	if in.Reason != nil {
		a.Reason = *in.Reason
	}
	if in.Owner != nil {
		a.Owner = *in.Owner
		a.Steps[0].Approver = *in.Owner
	}
	if in.OwnerManager != nil {
		a.OwnerManager = *in.OwnerManager
		a.Steps[1].Approver = *in.OwnerManager
	}
	a.UpdatedAt = time.Now()
	s.approvals[id] = a
	writeJSON(w, http.StatusOK, a)
}

// decide 处理当前节点的通过或驳回。审批人来自审批单上固定的 owner / owner_manager。
func (s *store) decide(w http.ResponseWriter, r *http.Request, decision string) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var in decisionInput
	if err := decodeOptionalJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "请求体不是合法的 JSON")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.approvals[id]
	if !ok {
		writeStoreError(w, ErrNotFound)
		return
	}
	if a.Status != StatusPending || a.CurrentStep >= len(a.Steps) {
		writeStoreError(w, ErrConflict)
		return
	}

	step := &a.Steps[a.CurrentStep]
	step.Status = decision
	step.Remark = in.Remark
	step.UpdatedAt = time.Now()
	a.UpdatedAt = time.Now()

	if decision == StepRejected {
		// 驳回：退回申请人，重置节点，保存驳回意见供申请人查看。
		a.Status = StatusReturned
		a.CurrentStep = 0
		a.LastRejectReason = in.Remark
		a.Steps = newSteps(a.Owner, a.OwnerManager)
	} else if a.CurrentStep == 0 {
		// 需求owner通过：流转到需求owner上级。
		a.CurrentStep = 1
	} else {
		// 上级通过：整体完成。
		a.CurrentStep = 2
		a.Status = StatusApproved
	}
	s.approvals[id] = a
	writeJSON(w, http.StatusOK, a)
}

// resubmit 申请人修改后重新提交，只有 returned 状态可用，回到第一节点。
func (s *store) resubmit(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.approvals[id]
	if !ok {
		writeStoreError(w, ErrNotFound)
		return
	}
	if a.Status != StatusReturned {
		writeStoreError(w, ErrConflict)
		return
	}
	a.Status = StatusPending
	a.CurrentStep = 0
	a.LastRejectReason = ""
	a.Steps = newSteps(a.Owner, a.OwnerManager)
	a.UpdatedAt = time.Now()
	s.approvals[id] = a
	writeJSON(w, http.StatusOK, a)
}

func (s *store) remove(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.approvals[id]; !ok {
		writeStoreError(w, ErrNotFound)
		return
	}
	delete(s.approvals, id)
	w.WriteHeader(http.StatusNoContent)
}

func newRouter(s *store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /approvals", s.list)
	mux.HandleFunc("POST /approvals", s.create)
	mux.HandleFunc("GET /approvals/{id}", s.get)
	mux.HandleFunc("PATCH /approvals/{id}", s.patch)
	mux.HandleFunc("POST /approvals/{id}/resubmit", s.resubmit)
	mux.HandleFunc("DELETE /approvals/{id}", s.remove)
	mux.HandleFunc("POST /approvals/{id}/approve", func(w http.ResponseWriter, r *http.Request) {
		s.decide(w, r, StepApproved)
	})
	mux.HandleFunc("POST /approvals/{id}/reject", func(w http.ResponseWriter, r *http.Request) {
		s.decide(w, r, StepRejected)
	})
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
	fmt.Printf("审批 API 正在监听端口 %s\n", strings.TrimPrefix(addr, ":"))
	log.Fatal(srv.ListenAndServe())
}
