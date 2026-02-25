package httpapp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"mini-atoms/internal/auth"
	"mini-atoms/internal/config"
	"mini-atoms/internal/store"
)

func TestHealthz(t *testing.T) {
	t.Parallel()

	h := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHomeRedirectsToProjects(t *testing.T) {
	t.Parallel()

	h := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got := rec.Header().Get("Location"); got != "/projects" {
		t.Fatalf("Location = %q, want %q", got, "/projects")
	}
}

func TestProjectsRedirectsWhenUnauthenticated(t *testing.T) {
	t.Parallel()

	h := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/projects", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got := rec.Header().Get("Location"); got != "/login" {
		t.Fatalf("Location = %q, want %q", got, "/login")
	}
}

func TestRegisterCreatesSessionAndAccessProjects(t *testing.T) {
	t.Parallel()

	h := newTestRouter(t)

	form := url.Values{}
	form.Set("email", "foo@example.com")
	form.Set("password", "password123")

	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("register status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got := rec.Header().Get("Location"); got != "/projects" {
		t.Fatalf("register Location = %q, want %q", got, "/projects")
	}

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie")
	}

	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == sessionCookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil || sessionCookie.Value == "" {
		t.Fatal("expected non-empty auth session cookie")
	}

	req2 := httptest.NewRequest(http.MethodGet, "/projects", nil)
	req2.AddCookie(sessionCookie)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("projects status = %d, want %d", rec2.Code, http.StatusOK)
	}
	body := rec2.Body.String()
	if !strings.Contains(body, "foo@example.com") {
		t.Fatalf("projects body missing email, body=%q", body)
	}
}

func TestLoginInvalidCredentials(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "auth-invalid.db")
	db, err := store.OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("gorm db(): %v", err)
	}
	defer sqlDB.Close()

	repo := store.NewAuthRepo(db)
	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if _, err := repo.CreateUser(ctx, "bar@example.com", hash); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	h, err := NewRouter(Dependencies{
		Config: config.Config{
			AppEnv:        "test",
			HTTPAddr:      ":0",
			DatabasePath:  dbPath,
			SessionSecret: "test-session-secret",
		},
		DB: db,
	})
	if err != nil {
		t.Fatalf("new router: %v", err)
	}

	form := url.Values{}
	form.Set("email", "bar@example.com")
	form.Set("password", "wrong-password")

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("login status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	body, _ := io.ReadAll(rec.Result().Body)
	if !strings.Contains(string(body), "邮箱或密码错误") {
		t.Fatalf("login body missing error message, body=%q", string(body))
	}
}

func TestCreateProjectAndOpenDetail(t *testing.T) {
	t.Parallel()

	h := newTestRouter(t)
	sessionCookie := registerAndLoginForTest(t, h, "m2-owner@example.com")

	form := url.Values{}
	form.Set("goal_prompt", "帮我做一个简历站点")

	req := httptest.NewRequest(http.MethodPost, "/projects", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create project status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	location := rec.Header().Get("Location")
	if !strings.HasPrefix(location, "/projects/") {
		t.Fatalf("create project Location = %q, want /projects/:slug", location)
	}

	req2 := httptest.NewRequest(http.MethodGet, location, nil)
	req2.AddCookie(sessionCookie)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want %d", rec2.Code, http.StatusOK)
	}
	body := rec2.Body.String()
	if !strings.Contains(body, "帮我做一个简历站点") {
		t.Fatalf("detail body missing goal prompt, body=%q", body)
	}
	if !strings.Contains(body, "聊天区") {
		t.Fatalf("detail body missing chat placeholder, body=%q", body)
	}
	if !strings.Contains(body, "项目预览器") {
		t.Fatalf("detail body missing preview placeholder, body=%q", body)
	}
	if !strings.Contains(body, `data-auto-generate="true"`) {
		t.Fatalf("detail body should enable auto generate on first open, body=%q", body)
	}
	if !strings.Contains(body, `aria-label="打开我的项目侧边栏"`) {
		t.Fatalf("detail body missing global my-projects sidebar trigger, body=%q", body)
	}
}

func TestGenerateDraftFromProjectDetail(t *testing.T) {
	t.Parallel()

	h := newTestRouter(t)
	sessionCookie := registerAndLoginForTest(t, h, "m4-generate@example.com")

	createForm := url.Values{}
	createForm.Set("goal_prompt", "帮我做一个待办应用")
	createReq := httptest.NewRequest(http.MethodPost, "/projects", strings.NewReader(createForm.Encode()))
	createReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	createReq.AddCookie(sessionCookie)
	createRec := httptest.NewRecorder()
	h.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusSeeOther {
		t.Fatalf("create project status = %d, want %d", createRec.Code, http.StatusSeeOther)
	}
	projectPath := createRec.Header().Get("Location")

	genForm := url.Values{}
	genForm.Set("prompt", "增加完成状态和数量统计")
	genReq := httptest.NewRequest(http.MethodPost, projectPath+"/generate", strings.NewReader(genForm.Encode()))
	genReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	genReq.AddCookie(sessionCookie)
	genRec := httptest.NewRecorder()
	h.ServeHTTP(genRec, genReq)
	if genRec.Code != http.StatusSeeOther {
		t.Fatalf("generate status = %d, want %d", genRec.Code, http.StatusSeeOther)
	}
	if got := genRec.Header().Get("Location"); got != projectPath {
		t.Fatalf("generate Location = %q, want %q", got, projectPath)
	}

	detailReq := httptest.NewRequest(http.MethodGet, projectPath, nil)
	detailReq.AddCookie(sessionCookie)
	detailRec := httptest.NewRecorder()
	h.ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want %d", detailRec.Code, http.StatusOK)
	}
	body := detailRec.Body.String()
	if !strings.Contains(body, "AI") {
		t.Fatalf("detail body missing assistant chat badge, body=%q", body)
	}
	if !strings.Contains(body, "已生成草稿") {
		t.Fatalf("detail body missing assistant summary, body=%q", body)
	}
	if strings.Contains(body, "调试信息") || strings.Contains(body, "草稿配置") {
		t.Fatalf("detail body should not expose debug config panel, body=%q", body)
	}
	if strings.Contains(body, `data-auto-generate="true"`) {
		t.Fatalf("detail body should not auto generate after draft exists, body=%q", body)
	}
	if strings.Contains(body, "M4 已接入") || strings.Contains(body, "P0 修复重试") || strings.Contains(body, "HTMX 局部刷新") {
		t.Fatalf("detail body should not contain dev milestone copy, body=%q", body)
	}
}

func TestGenerateDraftFromProjectDetailHTMXPartial(t *testing.T) {
	t.Parallel()

	h := newTestRouter(t)
	sessionCookie := registerAndLoginForTest(t, h, "m4-htmx@example.com")

	createForm := url.Values{}
	createForm.Set("goal_prompt", "帮我做一个待办应用")
	createReq := httptest.NewRequest(http.MethodPost, "/projects", strings.NewReader(createForm.Encode()))
	createReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	createReq.AddCookie(sessionCookie)
	createRec := httptest.NewRecorder()
	h.ServeHTTP(createRec, createReq)
	projectPath := createRec.Header().Get("Location")

	genForm := url.Values{}
	genForm.Set("prompt", "增加统计信息")
	genReq := httptest.NewRequest(http.MethodPost, projectPath+"/generate", strings.NewReader(genForm.Encode()))
	genReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	genReq.Header.Set("HX-Request", "true")
	genReq.AddCookie(sessionCookie)
	genRec := httptest.NewRecorder()
	h.ServeHTTP(genRec, genReq)

	if genRec.Code != http.StatusOK {
		t.Fatalf("HTMX generate status = %d, want %d", genRec.Code, http.StatusOK)
	}
	body := genRec.Body.String()
	if !strings.Contains(body, `id="project-workbench"`) {
		t.Fatalf("HTMX generate body missing workbench wrapper, body=%q", body)
	}
	if strings.Contains(body, "<html") {
		t.Fatalf("HTMX generate body should be partial, body=%q", body)
	}
	if !strings.Contains(body, "已生成草稿") {
		t.Fatalf("HTMX generate body missing assistant summary, body=%q", body)
	}
	if !strings.Contains(body, `x-on:htmx:before-swap.window="stopOnWorkbenchSwap($event)"`) {
		t.Fatalf("HTMX generate body missing ai generate stop fallback on swap, body=%q", body)
	}
}

func TestProjectPreviewCreateRecordHTMXPartial(t *testing.T) {
	t.Parallel()

	h := newTestRouter(t)
	sessionCookie := registerAndLoginForTest(t, h, "m5-preview-create@example.com")
	projectPath := createProjectAndGenerateDraftForTest(t, h, sessionCookie, "帮我做一个待办应用", "包含 form、list、toggle、stats")

	form := url.Values{}
	form.Set("title", "准备发布版本")
	form.Set("done", "0")
	form.Set("page_id", "home")
	form.Set("preview_mode", "editor")

	req := httptest.NewRequest(http.MethodPost, projectPath+"/preview/records/todos", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("preview create status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="project-preview-panel"`) {
		t.Fatalf("preview create body missing preview panel root, body=%q", body)
	}
	if !strings.Contains(body, "准备发布版本") {
		t.Fatalf("preview create body missing record title, body=%q", body)
	}
	if !strings.Contains(body, "总数") {
		t.Fatalf("preview create body missing stats label, body=%q", body)
	}
}

func TestProjectPreviewWriteForbiddenInShareReadonlyMode(t *testing.T) {
	t.Parallel()

	h := newTestRouter(t)
	sessionCookie := registerAndLoginForTest(t, h, "m5-preview-readonly@example.com")
	projectPath := createProjectAndGenerateDraftForTest(t, h, sessionCookie, "帮我做一个待办应用", "包含 form、list、toggle、stats")

	form := url.Values{}
	form.Set("title", "不应该写入")
	form.Set("done", "0")
	form.Set("page_id", "home")
	form.Set("preview_mode", "share_readonly")

	req := httptest.NewRequest(http.MethodPost, projectPath+"/preview/records/todos", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("readonly preview create status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	if !strings.Contains(rec.Body.String(), "readonly") {
		t.Fatalf("readonly preview create body missing readonly message, body=%q", rec.Body.String())
	}
}

func TestProjectPreviewUpdateAndDeleteRecordHTMXPartial(t *testing.T) {
	t.Parallel()

	h := newTestRouter(t)
	sessionCookie := registerAndLoginForTest(t, h, "m5-preview-update-delete@example.com")
	projectPath := createProjectAndGenerateDraftForTest(t, h, sessionCookie, "帮我做一个待办应用", "包含 form、list、toggle、stats")

	createForm := url.Values{}
	createForm.Set("title", "待修改任务")
	createForm.Set("done", "0")
	createForm.Set("page_id", "home")
	createForm.Set("preview_mode", "editor")
	createReq := httptest.NewRequest(http.MethodPost, projectPath+"/preview/records/todos", strings.NewReader(createForm.Encode()))
	createReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	createReq.Header.Set("HX-Request", "true")
	createReq.AddCookie(sessionCookie)
	createRec := httptest.NewRecorder()
	h.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("preview create status = %d, want %d", createRec.Code, http.StatusOK)
	}
	body := createRec.Body.String()
	if !strings.Contains(body, "待修改任务") {
		t.Fatalf("preview create body missing record title, body=%q", body)
	}
	recordID := firstRegexSubmatch(t, body, `/preview/records/todos/([0-9]+)`)
	if recordID == "" {
		t.Fatalf("failed to extract record id from body=%q", body)
	}

	updateForm := url.Values{}
	updateForm.Set("title", "已修改任务")
	updateForm.Set("done", "1")
	updateForm.Set("page_id", "home")
	updateForm.Set("preview_mode", "editor")
	updateReq := httptest.NewRequest(http.MethodPost, projectPath+"/preview/records/todos/"+recordID, strings.NewReader(updateForm.Encode()))
	updateReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	updateReq.Header.Set("HX-Request", "true")
	updateReq.AddCookie(sessionCookie)
	updateRec := httptest.NewRecorder()
	h.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("preview update status = %d, want %d", updateRec.Code, http.StatusOK)
	}
	updateBody := updateRec.Body.String()
	if !strings.Contains(updateBody, "已修改任务") {
		t.Fatalf("preview update body missing updated title, body=%q", updateBody)
	}
	if !strings.Contains(updateBody, "已完成") {
		t.Fatalf("preview update body missing updated bool text, body=%q", updateBody)
	}

	deleteForm := url.Values{}
	deleteForm.Set("page_id", "home")
	deleteForm.Set("preview_mode", "editor")
	deleteReq := httptest.NewRequest(http.MethodPost, projectPath+"/preview/records/todos/"+recordID+"/delete", strings.NewReader(deleteForm.Encode()))
	deleteReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	deleteReq.Header.Set("HX-Request", "true")
	deleteReq.AddCookie(sessionCookie)
	deleteRec := httptest.NewRecorder()
	h.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("preview delete status = %d, want %d", deleteRec.Code, http.StatusOK)
	}
	deleteBody := deleteRec.Body.String()
	if strings.Contains(deleteBody, "已修改任务") {
		t.Fatalf("preview delete body should not contain deleted title, body=%q", deleteBody)
	}
}

func TestPublishAndShareRoutesAndShareReadonlyWrites(t *testing.T) {
	t.Parallel()

	h := newTestRouter(t)
	sessionCookie := registerAndLoginForTest(t, h, "m7-publish-share@example.com")
	projectPath := createProjectAndGenerateDraftForTest(t, h, sessionCookie, "帮我做一个待办应用", "包含 form、list、toggle、stats")

	publishReq := httptest.NewRequest(http.MethodPost, projectPath+"/publish", nil)
	publishReq.AddCookie(sessionCookie)
	publishRec := httptest.NewRecorder()
	h.ServeHTTP(publishRec, publishReq)
	if publishRec.Code != http.StatusSeeOther {
		t.Fatalf("publish status = %d, want %d", publishRec.Code, http.StatusSeeOther)
	}
	publishedPath := publishRec.Header().Get("Location")
	if !strings.HasPrefix(publishedPath, "/p/") {
		t.Fatalf("publish Location = %q, want /p/:slug", publishedPath)
	}

	pubGetReq := httptest.NewRequest(http.MethodGet, publishedPath, nil)
	pubGetRec := httptest.NewRecorder()
	h.ServeHTTP(pubGetRec, pubGetReq)
	if pubGetRec.Code != http.StatusOK {
		t.Fatalf("GET published status = %d, want %d", pubGetRec.Code, http.StatusOK)
	}
	if !strings.Contains(pubGetRec.Body.String(), "项目预览器") {
		t.Fatalf("published body missing preview, body=%q", pubGetRec.Body.String())
	}
	if strings.Contains(strings.ToLower(pubGetRec.Body.String()), "readonly preview") {
		t.Fatalf("published body should not expose internal preview marker, body=%q", pubGetRec.Body.String())
	}

	shareReq := httptest.NewRequest(http.MethodPost, projectPath+"/share", nil)
	shareReq.AddCookie(sessionCookie)
	shareRec := httptest.NewRecorder()
	h.ServeHTTP(shareRec, shareReq)
	if shareRec.Code != http.StatusSeeOther {
		t.Fatalf("share status = %d, want %d", shareRec.Code, http.StatusSeeOther)
	}
	sharePath := shareRec.Header().Get("Location")
	if !strings.HasPrefix(sharePath, "/share/") {
		t.Fatalf("share Location = %q, want /share/:slug", sharePath)
	}

	shareGetReq := httptest.NewRequest(http.MethodGet, sharePath, nil)
	shareGetRec := httptest.NewRecorder()
	h.ServeHTTP(shareGetRec, shareGetReq)
	if shareGetRec.Code != http.StatusOK {
		t.Fatalf("GET share status = %d, want %d", shareGetRec.Code, http.StatusOK)
	}
	shareBody := shareGetRec.Body.String()
	if !strings.Contains(shareBody, "对话消息") {
		t.Fatalf("share body missing chat, body=%q", shareBody)
	}
	if !strings.Contains(shareBody, "只读分享") {
		t.Fatalf("share body missing readonly share marker, body=%q", shareBody)
	}
	if strings.Contains(strings.ToLower(shareBody), ">share_readonly<") {
		t.Fatalf("share body should not expose internal mode value in visible copy, body=%q", shareBody)
	}

	readonlyWriteForm := url.Values{}
	readonlyWriteForm.Set("page_id", "home")
	readonlyWriteForm.Set("preview_mode", "editor")
	readonlyWriteForm.Set("title", "forbidden")
	readonlyWriteForm.Set("done", "0")
	shareWriteReq := httptest.NewRequest(http.MethodPost, sharePath+"/records/todos", strings.NewReader(readonlyWriteForm.Encode()))
	shareWriteReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	shareWriteRec := httptest.NewRecorder()
	h.ServeHTTP(shareWriteRec, shareWriteReq)
	if shareWriteRec.Code != http.StatusForbidden {
		t.Fatalf("share write status = %d, want %d", shareWriteRec.Code, http.StatusForbidden)
	}
}

func TestPublishedPageAllowsWrites(t *testing.T) {
	t.Parallel()

	h := newTestRouter(t)
	sessionCookie := registerAndLoginForTest(t, h, "m7-published-write@example.com")
	projectPath := createProjectAndGenerateDraftForTest(t, h, sessionCookie, "帮我做一个待办应用", "包含 form、list、toggle、stats")

	publishReq := httptest.NewRequest(http.MethodPost, projectPath+"/publish", nil)
	publishReq.AddCookie(sessionCookie)
	publishRec := httptest.NewRecorder()
	h.ServeHTTP(publishRec, publishReq)
	if publishRec.Code != http.StatusSeeOther {
		t.Fatalf("publish status = %d, want %d", publishRec.Code, http.StatusSeeOther)
	}
	publishedPath := publishRec.Header().Get("Location")

	form := url.Values{}
	form.Set("title", "公开页写入")
	form.Set("done", "0")
	form.Set("page_id", "home")
	form.Set("preview_mode", "editor")
	req := httptest.NewRequest(http.MethodPost, publishedPath+"/records/todos", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("published write status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "公开页写入") {
		t.Fatalf("published write body missing created row, body=%q", rec.Body.String())
	}
}

func TestCreateProjectValidationError(t *testing.T) {
	t.Parallel()

	h := newTestRouter(t)
	sessionCookie := registerAndLoginForTest(t, h, "m2-validation@example.com")

	form := url.Values{}
	form.Set("goal_prompt", "")

	req := httptest.NewRequest(http.MethodPost, "/projects", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create project validation status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "请输入项目需求") {
		t.Fatalf("validation body missing prompt error, body=%q", body)
	}
}

func TestProjectDetailEmbedsPreviewInIframeAndChatOnLeft(t *testing.T) {
	t.Parallel()

	h := newTestRouter(t)
	email := "m8-iframe-layout@example.com"
	sessionCookie := registerAndLoginForTest(t, h, email)

	createForm := url.Values{}
	createForm.Set("goal_prompt", "帮我做一个简单看板")
	createReq := httptest.NewRequest(http.MethodPost, "/projects", strings.NewReader(createForm.Encode()))
	createReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	createReq.AddCookie(sessionCookie)
	createRec := httptest.NewRecorder()
	h.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusSeeOther {
		t.Fatalf("create project status = %d, want %d", createRec.Code, http.StatusSeeOther)
	}
	projectPath := createRec.Header().Get("Location")

	req := httptest.NewRequest(http.MethodGet, projectPath, nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "创建人 "+email) {
		t.Fatalf("detail body missing creator email, body=%q", body)
	}
	if !strings.Contains(body, `<iframe`) {
		t.Fatalf("detail body should embed preview iframe, body=%q", body)
	}
	if !strings.Contains(body, projectPath+`/preview/frame`) {
		t.Fatalf("detail body missing preview frame src, body=%q", body)
	}

	chatIdx := strings.Index(body, "聊天区")
	if chatIdx < 0 {
		t.Fatalf("detail body missing chat title, body=%q", body)
	}
	iframeIdx := strings.Index(body, "<iframe")
	if iframeIdx < 0 {
		t.Fatalf("detail body missing iframe, body=%q", body)
	}
	if chatIdx > iframeIdx {
		t.Fatalf("detail workbench should render chat before preview iframe for desktop left-right layout, body=%q", body)
	}
}

func TestProjectPreviewFramePageRendersIsolatedDocument(t *testing.T) {
	t.Parallel()

	h := newTestRouter(t)
	sessionCookie := registerAndLoginForTest(t, h, "m8-preview-frame@example.com")

	createForm := url.Values{}
	createForm.Set("goal_prompt", "帮我做一个计时器")
	createReq := httptest.NewRequest(http.MethodPost, "/projects", strings.NewReader(createForm.Encode()))
	createReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	createReq.AddCookie(sessionCookie)
	createRec := httptest.NewRecorder()
	h.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusSeeOther {
		t.Fatalf("create project status = %d, want %d", createRec.Code, http.StatusSeeOther)
	}
	projectPath := createRec.Header().Get("Location")

	req := httptest.NewRequest(http.MethodGet, projectPath+"/preview/frame", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("preview frame status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(strings.ToLower(body), "<!doctype html>") {
		t.Fatalf("preview frame should return full html document, body=%q", body)
	}
	if !strings.Contains(body, `id="project-preview-panel"`) {
		t.Fatalf("preview frame body missing preview panel root, body=%q", body)
	}
	if strings.Contains(body, `aria-label="打开我的项目侧边栏"`) {
		t.Fatalf("preview frame should hide outer page navigation chrome, body=%q", body)
	}
}

func firstRegexSubmatch(t *testing.T, s, pattern string) string {
	t.Helper()
	re := regexp.MustCompile(pattern)
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func createProjectAndGenerateDraftForTest(t *testing.T, h http.Handler, sessionCookie *http.Cookie, goalPrompt, genPrompt string) string {
	t.Helper()

	createForm := url.Values{}
	createForm.Set("goal_prompt", goalPrompt)
	createReq := httptest.NewRequest(http.MethodPost, "/projects", strings.NewReader(createForm.Encode()))
	createReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	createReq.AddCookie(sessionCookie)
	createRec := httptest.NewRecorder()
	h.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusSeeOther {
		t.Fatalf("create project status = %d, want %d", createRec.Code, http.StatusSeeOther)
	}
	projectPath := createRec.Header().Get("Location")

	genForm := url.Values{}
	genForm.Set("prompt", genPrompt)
	genReq := httptest.NewRequest(http.MethodPost, projectPath+"/generate", strings.NewReader(genForm.Encode()))
	genReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	genReq.AddCookie(sessionCookie)
	genRec := httptest.NewRecorder()
	h.ServeHTTP(genRec, genReq)
	if genRec.Code != http.StatusSeeOther {
		t.Fatalf("generate status = %d, want %d", genRec.Code, http.StatusSeeOther)
	}
	return projectPath
}

func registerAndLoginForTest(t *testing.T, h http.Handler, email string) *http.Cookie {
	t.Helper()

	form := url.Values{}
	form.Set("email", email)
	form.Set("password", "password123")

	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("register status = %d, want %d", rec.Code, http.StatusSeeOther)
	}

	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName {
			return c
		}
	}
	t.Fatal("expected auth session cookie")
	return nil
}

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "router.db")
	db, err := store.OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("gorm db(): %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	h, err := NewRouter(Dependencies{
		Config: config.Config{
			AppEnv:        "test",
			HTTPAddr:      ":0",
			DatabasePath:  dbPath,
			SessionSecret: "test-session-secret",
		},
		DB: db,
	})
	if err != nil {
		t.Fatalf("new router: %v", err)
	}

	return h
}
