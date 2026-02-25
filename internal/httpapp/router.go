package httpapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"mini-atoms/internal/apprender"
	"mini-atoms/internal/auth"
	"mini-atoms/internal/config"
	"mini-atoms/internal/generation"
	specpkg "mini-atoms/internal/spec"
	"mini-atoms/internal/store"
	webfs "mini-atoms/web"
)

const sessionCookieName = "mini_atoms_session"

type Dependencies struct {
	Config config.Config
	DB     *gorm.DB
}

type server struct {
	cfg             config.Config
	db              *gorm.DB
	authRepo        *store.AuthRepo
	projectRepo     *store.ProjectRepo
	collectionRepo  *store.CollectionRepo
	recordRepo      *store.RecordRepo
	chatRepo        *store.ChatRepo
	genService      *generation.Service
	genProviderName string
	templates       *template.Template
}

type templateData struct {
	Title           string
	ContentTemplate string
	RenderedContent template.HTML

	AppEnv           string
	HTTPAddr         string
	DatabasePath     string
	CurrentUserEmail string

	Error string

	FormEmail       string
	ProjectFormName string
	ProjectFormGoal string

	Projects               []projectCardView
	ShowcaseProjects       []projectCardView
	CurrentProject         *projectDetailView
	ChatMessages           []chatMessageView
	GenerateFormPrompt     string
	AutoGenerateOnLoad     bool
	ProjectDetailError     string
	GenerationProviderName string

	ShowChatComposer       bool
	ShowProjectActions     bool
	ShowBackToProjects     bool
	ShowDraftSpecPanel     bool
	ShowPublishedSpecPanel bool
	IsSharePage            bool
	IsPublishedPage        bool
	IsPreviewFramePage     bool

	PublishedPagePath string
	SharePagePath     string
	PublishActionPath string
	ShareActionPath   string

	PreviewApp            *apprender.AppView
	PreviewError          string
	PreviewPageID         string
	PreviewMode           string
	PreviewPageBasePath   string
	PreviewPanelPath      string
	PreviewFramePath      string
	PreviewActionBasePath string

	PreviewEditCollection string
	PreviewEditRecordID   int64
	PreviewEditValues     map[string]string
}

type projectCardView struct {
	Name          string
	Slug          string
	GoalPrompt    string
	UpdatedAtText string
}

type projectDetailView struct {
	ID                int64
	Name              string
	Slug              string
	GoalPrompt        string
	DraftSpecJSON     string
	PublishedSpecJSON string
	PublishedSlug     string
	ShareSlug         string
	PublishedAtText   string
	CreatedAtText     string
	UpdatedAtText     string
}

type chatMessageView struct {
	Role          string
	Content       string
	CreatedAtText string
	RoleLabel     string
	RoleClass     string
}

type previewUIState struct {
	PageID         string
	EditCollection string
	EditRecordID   int64
}

func NewRouter(deps Dependencies) (http.Handler, error) {
	tmpl, err := template.New("root").Funcs(template.FuncMap{
		"dict":     templateDict,
		"isTruthy": templateIsTruthy,
	}).ParseFS(webfs.FS, "templates/*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}

	s := &server{
		cfg:            deps.Config,
		db:             deps.DB,
		authRepo:       store.NewAuthRepo(deps.DB),
		projectRepo:    store.NewProjectRepo(deps.DB),
		collectionRepo: store.NewCollectionRepo(deps.DB),
		recordRepo:     store.NewRecordRepo(deps.DB),
		chatRepo:       store.NewChatRepo(deps.DB),
		templates:      tmpl,
	}
	genClient, genProviderName := buildGenerationClient(deps.Config)
	s.genProviderName = genProviderName
	s.genService = generation.NewService(generation.ServiceDeps{
		Projects: s.projectRepo,
		Chats:    s.chatRepo,
		Client:   genClient,
	})

	engine := gin.New()
	engine.Use(gin.Recovery())

	if staticFS, err := fs.Sub(webfs.FS, "static"); err == nil {
		engine.StaticFS("/static", http.FS(staticFS))
	}

	engine.GET("/", s.handleHome)
	engine.GET("/healthz", s.handleHealthz)
	engine.GET("/readyz", s.handleReadyz)
	engine.GET("/register", s.handleRegisterPage)
	engine.POST("/register", s.handleRegisterSubmit)
	engine.GET("/login", s.handleLoginPage)
	engine.POST("/login", s.handleLoginSubmit)
	engine.POST("/logout", s.handleLogoutSubmit)
	engine.GET("/p/:slug", s.handlePublishedProjectPage)
	engine.GET("/p/:slug/preview/frame", s.handlePublishedProjectPreviewFramePage)
	engine.GET("/p/:slug/preview", s.handlePublishedProjectPreviewPanel)
	engine.POST("/p/:slug/records/:collection", s.handlePublishedPreviewCreateRecordSubmit)
	engine.POST("/p/:slug/records/:collection/:recordID", s.handlePublishedPreviewUpdateRecordSubmit)
	engine.POST("/p/:slug/records/:collection/:recordID/delete", s.handlePublishedPreviewDeleteRecordSubmit)
	engine.POST("/p/:slug/toggle/:collection/:recordID/:field", s.handlePublishedPreviewToggleRecordSubmit)
	engine.GET("/share/:slug", s.handleSharedProjectPage)
	engine.GET("/share/:slug/preview/frame", s.handleSharedProjectPreviewFramePage)
	engine.GET("/share/:slug/preview", s.handleSharedProjectPreviewPanel)
	engine.POST("/share/:slug/records/:collection", s.handleSharedPreviewWriteForbidden)
	engine.POST("/share/:slug/records/:collection/:recordID", s.handleSharedPreviewWriteForbidden)
	engine.POST("/share/:slug/records/:collection/:recordID/delete", s.handleSharedPreviewWriteForbidden)
	engine.POST("/share/:slug/toggle/:collection/:recordID/:field", s.handleSharedPreviewWriteForbidden)

	authorized := engine.Group("/")
	authorized.Use(s.requireAuth())
	authorized.GET("/projects", s.handleProjects)
	authorized.POST("/projects", s.handleCreateProjectSubmit)
	authorized.GET("/projects/:slug", s.handleProjectDetail)
	authorized.POST("/projects/:slug/generate", s.handleGenerateProjectDraftSubmit)
	authorized.GET("/projects/:slug/preview/frame", s.handleProjectPreviewFramePage)
	authorized.GET("/projects/:slug/preview", s.handleProjectPreviewPanel)
	authorized.POST("/projects/:slug/preview/records/:collection", s.handlePreviewCreateRecordSubmit)
	authorized.POST("/projects/:slug/preview/records/:collection/:recordID", s.handlePreviewUpdateRecordSubmit)
	authorized.POST("/projects/:slug/preview/records/:collection/:recordID/delete", s.handlePreviewDeleteRecordSubmit)
	authorized.POST("/projects/:slug/preview/toggle/:collection/:recordID/:field", s.handlePreviewToggleRecordSubmit)
	authorized.POST("/projects/:slug/publish", s.handlePublishProjectSubmit)
	authorized.POST("/projects/:slug/share", s.handleShareProjectSubmit)

	return engine, nil
}

func templateDict(values ...any) (map[string]any, error) {
	if len(values)%2 != 0 {
		return nil, fmt.Errorf("dict requires even number of args")
	}
	out := make(map[string]any, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict key must be string")
		}
		out[key] = values[i+1]
	}
	return out, nil
}

func templateIsTruthy(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		s := strings.TrimSpace(strings.ToLower(x))
		return s == "1" || s == "true" || s == "on" || s == "yes"
	case int:
		return x != 0
	case int64:
		return x != 0
	case float64:
		return x != 0
	default:
		return false
	}
}

func (s *server) handleHome(c *gin.Context) {
	c.Redirect(http.StatusSeeOther, "/projects")
}

func (s *server) handleHealthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *server) handleReadyz(c *gin.Context) {
	sqlDB, err := s.db.DB()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "error": "db unavailable"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 1*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}

func (s *server) handleRegisterPage(c *gin.Context) {
	user, err := s.resolveCurrentUser(c)
	if err != nil {
		c.String(http.StatusInternalServerError, "load session failed")
		return
	}
	if user != nil {
		c.Redirect(http.StatusSeeOther, "/projects")
		return
	}
	s.renderRegisterPage(c, http.StatusOK, "", "")
}

func (s *server) handleRegisterSubmit(c *gin.Context) {
	email, err := auth.NormalizeAndValidateEmail(c.PostForm("email"))
	if err != nil {
		s.renderRegisterPage(c, http.StatusBadRequest, c.PostForm("email"), "请输入合法邮箱")
		return
	}

	passwordHash, err := auth.HashPassword(c.PostForm("password"))
	if err != nil {
		s.renderRegisterPage(c, http.StatusBadRequest, email, "密码长度至少 8 位")
		return
	}

	user, err := s.authRepo.CreateUser(c.Request.Context(), email, passwordHash)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			s.renderRegisterPage(c, http.StatusConflict, email, "该邮箱已注册")
			return
		}
		c.String(http.StatusInternalServerError, "create user failed")
		return
	}

	if err := s.startSession(c, user.ID); err != nil {
		c.String(http.StatusInternalServerError, "create session failed")
		return
	}

	c.Redirect(http.StatusSeeOther, "/projects")
}

func (s *server) handleLoginPage(c *gin.Context) {
	user, err := s.resolveCurrentUser(c)
	if err != nil {
		c.String(http.StatusInternalServerError, "load session failed")
		return
	}
	if user != nil {
		c.Redirect(http.StatusSeeOther, "/projects")
		return
	}
	s.renderLoginPage(c, http.StatusOK, "", "")
}

func (s *server) handleLoginSubmit(c *gin.Context) {
	email, err := auth.NormalizeAndValidateEmail(c.PostForm("email"))
	if err != nil {
		s.renderLoginPage(c, http.StatusBadRequest, c.PostForm("email"), "请输入合法邮箱")
		return
	}

	user, err := s.authRepo.GetUserByEmail(c.Request.Context(), email)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.renderLoginPage(c, http.StatusUnauthorized, email, "邮箱或密码错误")
			return
		}
		c.String(http.StatusInternalServerError, "load user failed")
		return
	}

	if !auth.CheckPassword(user.PasswordHash, c.PostForm("password")) {
		s.renderLoginPage(c, http.StatusUnauthorized, email, "邮箱或密码错误")
		return
	}

	if err := s.startSession(c, user.ID); err != nil {
		c.String(http.StatusInternalServerError, "create session failed")
		return
	}

	c.Redirect(http.StatusSeeOther, "/projects")
}

func (s *server) handleLogoutSubmit(c *gin.Context) {
	if token, err := c.Cookie(sessionCookieName); err == nil && token != "" {
		_ = s.authRepo.DeleteSessionByToken(c.Request.Context(), token)
	}
	s.clearSessionCookie(c)
	c.Redirect(http.StatusSeeOther, "/login")
}

func (s *server) handleProjects(c *gin.Context) {
	user, ok := s.currentUserFromContext(c)
	if !ok {
		return
	}

	s.renderProjectsPage(c, http.StatusOK, user, "", "", "", "")
}

func (s *server) handleCreateProjectSubmit(c *gin.Context) {
	user, ok := s.currentUserFromContext(c)
	if !ok {
		return
	}

	goalPrompt := strings.TrimSpace(c.PostForm("goal_prompt"))

	switch {
	case goalPrompt == "":
		s.renderProjectsPage(c, http.StatusBadRequest, user, "请输入项目需求", "", c.PostForm("goal_prompt"), "")
		return
	}

	project, err := s.projectRepo.CreateProject(c.Request.Context(), user.ID, "", goalPrompt)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			s.renderProjectsPage(c, http.StatusConflict, user, "项目创建失败，请重试", "", c.PostForm("goal_prompt"), "")
			return
		}
		c.String(http.StatusInternalServerError, "create project failed")
		return
	}

	c.Redirect(http.StatusSeeOther, "/projects/"+project.Slug)
}

func (s *server) handleProjectDetail(c *gin.Context) {
	user, ok := s.currentUserFromContext(c)
	if !ok {
		return
	}

	s.renderProjectDetailPage(c, http.StatusOK, user, c.Param("slug"), "", "", previewUIStateFromRequest(c))
}

func (s *server) handleGenerateProjectDraftSubmit(c *gin.Context) {
	user, ok := s.currentUserFromContext(c)
	if !ok {
		return
	}

	slug := c.Param("slug")
	project, err := s.projectRepo.GetProjectByUserAndSlug(c.Request.Context(), user.ID, slug)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.String(http.StatusNotFound, "project not found")
			return
		}
		c.String(http.StatusInternalServerError, "load project failed")
		return
	}

	prompt := strings.TrimSpace(c.PostForm("prompt"))
	if prompt == "" {
		if isHTMXRequest(c) {
			s.renderProjectWorkbenchPartial(c, http.StatusOK, user, project.Slug, "请输入本轮需求", "", previewUIStateFromRequest(c))
			return
		}
		s.renderProjectDetailPage(c, http.StatusBadRequest, user, project.Slug, "请输入本轮需求", "", previewUIStateFromRequest(c))
		return
	}

	isHTMX := isHTMXRequest(c)
	startedAt := time.Now()
	log.Printf("generate draft request started: user_id=%d project_id=%d slug=%s provider=%s htmx=%t prompt_chars=%d", user.ID, project.ID, project.Slug, s.genProviderName, isHTMX, len([]rune(prompt)))

	result, err := s.genService.GenerateDraft(c.Request.Context(), generation.GenerateDraftInput{
		UserID:     user.ID,
		ProjectID:  project.ID,
		UserPrompt: prompt,
	})
	if err != nil {
		log.Printf("generate draft request failed: user_id=%d project_id=%d slug=%s provider=%s htmx=%t duration_ms=%d err=%v", user.ID, project.ID, project.Slug, s.genProviderName, isHTMX, time.Since(startedAt).Milliseconds(), err)
		if isHTMX {
			s.renderProjectWorkbenchPartial(c, http.StatusOK, user, project.Slug, "生成失败："+err.Error(), prompt, previewUIStateFromRequest(c))
			return
		}
		s.renderProjectDetailPage(c, http.StatusBadRequest, user, project.Slug, "生成失败："+err.Error(), prompt, previewUIStateFromRequest(c))
		return
	}
	log.Printf("generate draft request succeeded: user_id=%d project_id=%d slug=%s provider=%s htmx=%t duration_ms=%d round_no=%d draft_bytes=%d", user.ID, project.ID, project.Slug, s.genProviderName, isHTMX, time.Since(startedAt).Milliseconds(), result.RoundNo, len(result.DraftSpecJSON))

	if isHTMX {
		s.renderProjectWorkbenchPartial(c, http.StatusOK, user, project.Slug, "", "", previewUIStateFromRequest(c))
		return
	}
	c.Redirect(http.StatusSeeOther, "/projects/"+project.Slug+buildPreviewQueryString(previewUIStateFromRequest(c), false))
}

func (s *server) handleProjectPreviewPanel(c *gin.Context) {
	user, ok := s.currentUserFromContext(c)
	if !ok {
		return
	}
	s.renderProjectPreviewPanelPartial(c, http.StatusOK, user, c.Param("slug"), previewUIStateFromRequest(c))
}

func (s *server) handleProjectPreviewFramePage(c *gin.Context) {
	user, ok := s.currentUserFromContext(c)
	if !ok {
		return
	}
	s.renderProjectPreviewFramePage(c, http.StatusOK, user, c.Param("slug"), previewUIStateFromRequest(c))
}

func (s *server) handlePublishProjectSubmit(c *gin.Context) {
	user, ok := s.currentUserFromContext(c)
	if !ok {
		return
	}

	project, err := s.projectRepo.PublishProjectByUserAndSlug(c.Request.Context(), user.ID, c.Param("slug"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.String(http.StatusNotFound, "project not found")
			return
		}
		s.renderProjectDetailPage(c, http.StatusBadRequest, user, c.Param("slug"), "发布失败："+err.Error(), "", previewUIStateFromRequest(c))
		return
	}
	c.Redirect(http.StatusSeeOther, "/p/"+project.PublishedSlug)
}

func (s *server) handleShareProjectSubmit(c *gin.Context) {
	user, ok := s.currentUserFromContext(c)
	if !ok {
		return
	}

	project, err := s.projectRepo.EnsureShareSlugByUserAndSlug(c.Request.Context(), user.ID, c.Param("slug"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.String(http.StatusNotFound, "project not found")
			return
		}
		s.renderProjectDetailPage(c, http.StatusBadRequest, user, c.Param("slug"), "生成分享链接失败："+err.Error(), "", previewUIStateFromRequest(c))
		return
	}
	c.Redirect(http.StatusSeeOther, "/share/"+project.ShareSlug)
}

func (s *server) handlePreviewCreateRecordSubmit(c *gin.Context) {
	user, ok := s.currentUserFromContext(c)
	if !ok {
		return
	}
	uiState := previewUIStateFromRequest(c)

	if apprender.ParseMode(c.PostForm("preview_mode")).IsReadOnly() {
		c.String(http.StatusForbidden, "preview readonly mode forbids write operations")
		return
	}

	project, appSpec, pageID, err := s.loadProjectDraftSpec(c, user.ID, c.Param("slug"), uiState.PageID)
	if err != nil {
		s.handleProjectDetailRenderError(c, err)
		return
	}

	if err := s.collectionRepo.SyncCollectionsFromSpec(c.Request.Context(), project.ID, appSpec); err != nil {
		uiState.PageID = pageID
		s.renderPreviewWriteError(c, user, project.Slug, uiState, "同步集合失败："+err.Error())
		return
	}

	collection, err := s.collectionRepo.GetCollectionByProjectAndName(c.Request.Context(), project.ID, c.Param("collection"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			uiState.PageID = pageID
			s.renderPreviewWriteError(c, user, project.Slug, uiState, "集合不存在")
			return
		}
		s.handleProjectDetailRenderError(c, err)
		return
	}

	inputs := make(map[string]string)
	if err := c.Request.ParseForm(); err != nil {
		uiState.PageID = pageID
		s.renderPreviewWriteError(c, user, project.Slug, uiState, "读取表单失败")
		return
	}
	for key, vals := range c.Request.PostForm {
		if key == "page_id" || key == "preview_mode" {
			continue
		}
		if len(vals) == 0 {
			continue
		}
		inputs[key] = vals[len(vals)-1]
	}

	if _, err := s.recordRepo.CreateRecord(c.Request.Context(), project.ID, collection, inputs); err != nil {
		uiState.PageID = pageID
		s.renderPreviewWriteError(c, user, project.Slug, uiState, "创建记录失败："+err.Error())
		return
	}

	if isHTMXRequest(c) {
		uiState.PageID = pageID
		uiState.EditCollection = ""
		uiState.EditRecordID = 0
		s.renderProjectPreviewPanelPartial(c, http.StatusOK, user, project.Slug, uiState)
		return
	}
	uiState.PageID = pageID
	c.Redirect(http.StatusSeeOther, "/projects/"+project.Slug+buildPreviewQueryString(uiState, false))
}

func (s *server) handlePreviewUpdateRecordSubmit(c *gin.Context) {
	user, ok := s.currentUserFromContext(c)
	if !ok {
		return
	}
	uiState := previewUIStateFromRequest(c)

	if apprender.ParseMode(c.PostForm("preview_mode")).IsReadOnly() {
		c.String(http.StatusForbidden, "preview readonly mode forbids write operations")
		return
	}

	project, appSpec, pageID, err := s.loadProjectDraftSpec(c, user.ID, c.Param("slug"), uiState.PageID)
	if err != nil {
		s.handleProjectDetailRenderError(c, err)
		return
	}
	if err := s.collectionRepo.SyncCollectionsFromSpec(c.Request.Context(), project.ID, appSpec); err != nil {
		uiState.PageID = pageID
		s.renderPreviewWriteError(c, user, project.Slug, uiState, "同步集合失败："+err.Error())
		return
	}
	collection, err := s.collectionRepo.GetCollectionByProjectAndName(c.Request.Context(), project.ID, c.Param("collection"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			uiState.PageID = pageID
			s.renderPreviewWriteError(c, user, project.Slug, uiState, "集合不存在")
			return
		}
		s.handleProjectDetailRenderError(c, err)
		return
	}
	recordID, err := strconv.ParseInt(strings.TrimSpace(c.Param("recordID")), 10, 64)
	if err != nil || recordID <= 0 {
		uiState.PageID = pageID
		s.renderPreviewWriteError(c, user, project.Slug, uiState, "记录 ID 无效")
		return
	}
	inputs, parseErr := previewRecordInputMapFromRequest(c)
	if parseErr != nil {
		uiState.PageID = pageID
		uiState.EditCollection = collection.Name
		uiState.EditRecordID = recordID
		s.renderPreviewWriteError(c, user, project.Slug, uiState, "读取表单失败")
		return
	}
	if _, err := s.recordRepo.UpdateRecord(c.Request.Context(), project.ID, collection, recordID, inputs); err != nil {
		uiState.PageID = pageID
		uiState.EditCollection = collection.Name
		uiState.EditRecordID = recordID
		s.renderPreviewWriteError(c, user, project.Slug, uiState, "更新记录失败："+err.Error())
		return
	}
	uiState.PageID = pageID
	uiState.EditCollection = ""
	uiState.EditRecordID = 0
	if isHTMXRequest(c) {
		s.renderProjectPreviewPanelPartial(c, http.StatusOK, user, project.Slug, uiState)
		return
	}
	c.Redirect(http.StatusSeeOther, "/projects/"+project.Slug+buildPreviewQueryString(uiState, false))
}

func (s *server) handlePreviewDeleteRecordSubmit(c *gin.Context) {
	user, ok := s.currentUserFromContext(c)
	if !ok {
		return
	}
	uiState := previewUIStateFromRequest(c)

	if apprender.ParseMode(c.PostForm("preview_mode")).IsReadOnly() {
		c.String(http.StatusForbidden, "preview readonly mode forbids write operations")
		return
	}

	project, appSpec, pageID, err := s.loadProjectDraftSpec(c, user.ID, c.Param("slug"), uiState.PageID)
	if err != nil {
		s.handleProjectDetailRenderError(c, err)
		return
	}
	if err := s.collectionRepo.SyncCollectionsFromSpec(c.Request.Context(), project.ID, appSpec); err != nil {
		uiState.PageID = pageID
		s.renderPreviewWriteError(c, user, project.Slug, uiState, "同步集合失败："+err.Error())
		return
	}
	collection, err := s.collectionRepo.GetCollectionByProjectAndName(c.Request.Context(), project.ID, c.Param("collection"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			uiState.PageID = pageID
			s.renderPreviewWriteError(c, user, project.Slug, uiState, "集合不存在")
			return
		}
		s.handleProjectDetailRenderError(c, err)
		return
	}
	recordID, err := strconv.ParseInt(strings.TrimSpace(c.Param("recordID")), 10, 64)
	if err != nil || recordID <= 0 {
		uiState.PageID = pageID
		s.renderPreviewWriteError(c, user, project.Slug, uiState, "记录 ID 无效")
		return
	}
	if err := s.recordRepo.DeleteRecord(c.Request.Context(), project.ID, collection.ID, recordID); err != nil {
		uiState.PageID = pageID
		s.renderPreviewWriteError(c, user, project.Slug, uiState, "删除记录失败："+err.Error())
		return
	}
	uiState.PageID = pageID
	if uiState.EditCollection == collection.Name && uiState.EditRecordID == recordID {
		uiState.EditCollection = ""
		uiState.EditRecordID = 0
	}
	if isHTMXRequest(c) {
		s.renderProjectPreviewPanelPartial(c, http.StatusOK, user, project.Slug, uiState)
		return
	}
	c.Redirect(http.StatusSeeOther, "/projects/"+project.Slug+buildPreviewQueryString(uiState, false))
}

func (s *server) handlePreviewToggleRecordSubmit(c *gin.Context) {
	user, ok := s.currentUserFromContext(c)
	if !ok {
		return
	}
	uiState := previewUIStateFromRequest(c)

	if apprender.ParseMode(c.PostForm("preview_mode")).IsReadOnly() {
		c.String(http.StatusForbidden, "preview readonly mode forbids write operations")
		return
	}

	project, appSpec, pageID, err := s.loadProjectDraftSpec(c, user.ID, c.Param("slug"), uiState.PageID)
	if err != nil {
		s.handleProjectDetailRenderError(c, err)
		return
	}

	if err := s.collectionRepo.SyncCollectionsFromSpec(c.Request.Context(), project.ID, appSpec); err != nil {
		uiState.PageID = pageID
		s.renderPreviewWriteError(c, user, project.Slug, uiState, "同步集合失败："+err.Error())
		return
	}

	collection, err := s.collectionRepo.GetCollectionByProjectAndName(c.Request.Context(), project.ID, c.Param("collection"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			uiState.PageID = pageID
			s.renderPreviewWriteError(c, user, project.Slug, uiState, "集合不存在")
			return
		}
		s.handleProjectDetailRenderError(c, err)
		return
	}

	recordID, err := strconv.ParseInt(strings.TrimSpace(c.Param("recordID")), 10, 64)
	if err != nil || recordID <= 0 {
		uiState.PageID = pageID
		s.renderPreviewWriteError(c, user, project.Slug, uiState, "记录 ID 无效")
		return
	}

	if _, err := s.recordRepo.ToggleBoolField(c.Request.Context(), project.ID, collection, recordID, c.Param("field")); err != nil {
		uiState.PageID = pageID
		s.renderPreviewWriteError(c, user, project.Slug, uiState, "切换状态失败："+err.Error())
		return
	}

	if isHTMXRequest(c) {
		uiState.PageID = pageID
		s.renderProjectPreviewPanelPartial(c, http.StatusOK, user, project.Slug, uiState)
		return
	}
	uiState.PageID = pageID
	c.Redirect(http.StatusSeeOther, "/projects/"+project.Slug+buildPreviewQueryString(uiState, false))
}

func (s *server) requireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, err := s.resolveCurrentUser(c)
		if err != nil {
			c.String(http.StatusInternalServerError, "load session failed")
			c.Abort()
			return
		}
		if user == nil {
			c.Redirect(http.StatusSeeOther, "/login")
			c.Abort()
			return
		}

		c.Set("current_user", *user)
		c.Next()
	}
}

func (s *server) resolveCurrentUser(c *gin.Context) (*store.User, error) {
	token, err := c.Cookie(sessionCookieName)
	if err != nil {
		if errors.Is(err, http.ErrNoCookie) {
			return nil, nil
		}
		return nil, fmt.Errorf("read session cookie: %w", err)
	}
	if strings.TrimSpace(token) == "" {
		s.clearSessionCookie(c)
		return nil, nil
	}

	userSession, err := s.authRepo.GetUserBySessionToken(c.Request.Context(), token)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.clearSessionCookie(c)
			return nil, nil
		}
		return nil, fmt.Errorf("load session user: %w", err)
	}

	if auth.IsSessionExpired(time.Now(), userSession.Session.ExpiresAt) {
		_ = s.authRepo.DeleteSessionByToken(c.Request.Context(), token)
		s.clearSessionCookie(c)
		return nil, nil
	}

	user := userSession.User
	return &user, nil
}

func (s *server) currentUserFromContext(c *gin.Context) (store.User, bool) {
	userVal, ok := c.Get("current_user")
	if !ok {
		c.Redirect(http.StatusSeeOther, "/login")
		return store.User{}, false
	}
	user, ok := userVal.(store.User)
	if !ok {
		c.String(http.StatusInternalServerError, "invalid auth context")
		return store.User{}, false
	}
	return user, true
}

func (s *server) startSession(c *gin.Context, userID int64) error {
	exp := auth.NewSessionExpiry(time.Now())

	var lastErr error
	for range 3 {
		token, err := auth.NewSessionToken()
		if err != nil {
			return err
		}
		err = s.authRepo.CreateSession(c.Request.Context(), userID, token, exp)
		if err == nil {
			c.SetSameSite(http.SameSiteLaxMode)
			c.SetCookie(
				sessionCookieName,
				token,
				int(auth.SessionTTL.Seconds()),
				"/",
				"",
				strings.EqualFold(s.cfg.AppEnv, "production"),
				true,
			)
			return nil
		}
		if !errors.Is(err, store.ErrConflict) {
			return err
		}
		lastErr = err
	}

	if lastErr != nil {
		return fmt.Errorf("create session retries exhausted: %w", lastErr)
	}
	return fmt.Errorf("create session retries exhausted")
}

func (s *server) clearSessionCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookieName, "", -1, "/", "", strings.EqualFold(s.cfg.AppEnv, "production"), true)
}

func (s *server) renderRegisterPage(c *gin.Context, status int, email, errMsg string) {
	s.renderTemplate(c, status, templateData{
		Title:           "Register - mini-atoms",
		ContentTemplate: "register_content",
		FormEmail:       email,
		Error:           errMsg,
	})
}

func (s *server) renderLoginPage(c *gin.Context, status int, email, errMsg string) {
	s.renderTemplate(c, status, templateData{
		Title:           "Login - mini-atoms",
		ContentTemplate: "login_content",
		FormEmail:       email,
		Error:           errMsg,
	})
}

func (s *server) renderProjectsPage(c *gin.Context, status int, user store.User, errMsg, formName, formGoal, selectedSlug string) {
	projects, err := s.projectRepo.ListProjectsByUser(c.Request.Context(), user.ID)
	if err != nil {
		c.String(http.StatusInternalServerError, "list projects failed")
		return
	}
	showcaseProjects, err := s.projectRepo.ListShowcaseProjects(c.Request.Context(), 6)
	if err != nil {
		c.String(http.StatusInternalServerError, "list showcase projects failed")
		return
	}

	_ = selectedSlug // M2 预留，后续可用于高亮当前项目

	data := templateData{
		Title:            "Projects - mini-atoms",
		ContentTemplate:  "projects_content",
		CurrentUserEmail: user.Email,
		Error:            errMsg,
		ProjectFormName:  formName,
		ProjectFormGoal:  formGoal,
		Projects:         toProjectCardViews(projects),
		ShowcaseProjects: toProjectCardViews(showcaseProjects),
	}
	s.renderTemplate(c, status, data)
}

func (s *server) renderProjectDetailPage(c *gin.Context, status int, user store.User, slug, errMsg, formPrompt string, uiState previewUIState) {
	data, err := s.buildProjectDetailTemplateData(c, user, slug, errMsg, formPrompt, uiState, "")
	if err != nil {
		s.handleProjectDetailRenderError(c, err)
		return
	}
	projects, err := s.projectRepo.ListProjectsByUser(c.Request.Context(), user.ID)
	if err != nil {
		c.String(http.StatusInternalServerError, "list projects failed")
		return
	}
	data.Projects = toProjectCardViews(projects)
	data.ContentTemplate = "project_detail_content"
	s.renderTemplate(c, status, data)
}

func (s *server) renderProjectWorkbenchPartial(c *gin.Context, status int, user store.User, slug, errMsg, formPrompt string, uiState previewUIState) {
	data, err := s.buildProjectDetailTemplateData(c, user, slug, errMsg, formPrompt, uiState, "")
	if err != nil {
		s.handleProjectDetailRenderError(c, err)
		return
	}
	data.ContentTemplate = "project_workbench_content"
	s.renderContentTemplate(c, status, data)
}

func (s *server) renderProjectPreviewPanelPartial(c *gin.Context, status int, user store.User, slug string, uiState previewUIState) {
	data, err := s.buildProjectDetailTemplateData(c, user, slug, "", "", uiState, "")
	if err != nil {
		s.handleProjectDetailRenderError(c, err)
		return
	}
	data.ContentTemplate = "project_detail_preview_panel"
	s.renderContentTemplate(c, status, data)
}

func (s *server) renderProjectPreviewFramePage(c *gin.Context, status int, user store.User, slug string, uiState previewUIState) {
	data, err := s.buildProjectDetailTemplateData(c, user, slug, "", "", uiState, "")
	if err != nil {
		s.handleProjectDetailRenderError(c, err)
		return
	}
	data.IsPreviewFramePage = true
	data.ContentTemplate = "project_preview_frame_content"
	s.renderTemplate(c, status, data)
}

func (s *server) renderPreviewWriteError(c *gin.Context, user store.User, slug string, uiState previewUIState, msg string) {
	if isHTMXRequest(c) {
		data, err := s.buildProjectDetailTemplateData(c, user, slug, "", "", uiState, msg)
		if err != nil {
			s.handleProjectDetailRenderError(c, err)
			return
		}
		data.ContentTemplate = "project_detail_preview_panel"
		s.renderContentTemplate(c, http.StatusBadRequest, data)
		return
	}
	s.renderProjectDetailPage(c, http.StatusBadRequest, user, slug, "", "", uiState)
}

func (s *server) buildProjectDetailTemplateData(c *gin.Context, user store.User, slug, errMsg, formPrompt string, uiState previewUIState, previewErr string) (templateData, error) {
	project, err := s.projectRepo.GetProjectByUserAndSlug(c.Request.Context(), user.ID, slug)
	if err != nil {
		return templateData{}, err
	}

	messages, err := s.chatRepo.ListMessagesByProject(c.Request.Context(), project.ID)
	if err != nil {
		return templateData{}, err
	}

	data := templateData{
		Title:                  project.Name + " - mini-atoms",
		CurrentUserEmail:       user.Email,
		CurrentProject:         toProjectDetailView(project),
		ChatMessages:           toChatMessageViews(messages),
		ProjectDetailError:     errMsg,
		GenerationProviderName: s.genProviderName,
		ShowChatComposer:       true,
		ShowProjectActions:     true,
		ShowBackToProjects:     true,
		ShowDraftSpecPanel:     true,
		ShowPublishedSpecPanel: true,
		PublishActionPath:      "/projects/" + project.Slug + "/publish",
		ShareActionPath:        "/projects/" + project.Slug + "/share",
		PublishedPagePath:      publishedPagePath(project),
		SharePagePath:          sharePagePath(project),
		PreviewPageID:          strings.TrimSpace(uiState.PageID),
		PreviewMode:            string(apprender.ModeEditor),
		PreviewPageBasePath:    "/projects/" + project.Slug,
		PreviewPanelPath:       "/projects/" + project.Slug + "/preview",
		PreviewFramePath:       "/projects/" + project.Slug + "/preview/frame",
		PreviewActionBasePath:  "/projects/" + project.Slug + "/preview",
		PreviewError:           previewErr,
		PreviewEditCollection:  strings.TrimSpace(uiState.EditCollection),
		PreviewEditRecordID:    uiState.EditRecordID,
	}

	if shouldAutoGenerateDraftOnFirstOpen(project, messages, formPrompt, errMsg) {
		data.AutoGenerateOnLoad = true
		data.GenerateFormPrompt = project.GoalPrompt
	} else {
		data.GenerateFormPrompt = formPrompt
	}

	if previewView, previewBuildErr := s.buildDraftPreviewAppView(c.Request.Context(), project, data.PreviewPageID, apprender.ModeEditor); previewBuildErr != nil {
		if data.PreviewError == "" {
			data.PreviewError = previewBuildErr.Error()
		}
	} else {
		data.PreviewApp = previewView
		if data.PreviewPageID == "" && previewView != nil {
			data.PreviewPageID = previewView.CurrentPage.ID
		}
	}

	if data.PreviewEditRecordID > 0 && data.PreviewEditCollection != "" {
		if values, valueErr := s.loadRecordEditValues(c.Request.Context(), project.ID, data.PreviewEditCollection, data.PreviewEditRecordID); valueErr != nil {
			if data.PreviewError == "" {
				data.PreviewError = "加载编辑记录失败：" + valueErr.Error()
			}
		} else {
			data.PreviewEditValues = values
		}
	}

	return data, nil
}

func shouldAutoGenerateDraftOnFirstOpen(project store.Project, messages []store.ChatMessage, formPrompt, errMsg string) bool {
	if strings.TrimSpace(errMsg) != "" {
		return false
	}
	if strings.TrimSpace(formPrompt) != "" {
		return false
	}
	if strings.TrimSpace(project.DraftSpecJSON) != "" {
		return false
	}
	return len(messages) == 0 && strings.TrimSpace(project.GoalPrompt) != ""
}

func (s *server) handleProjectDetailRenderError(c *gin.Context, err error) {
	if errors.Is(err, store.ErrNotFound) {
		c.String(http.StatusNotFound, "project not found")
		return
	}
	c.String(http.StatusInternalServerError, "load project detail failed")
}

func (s *server) renderTemplate(c *gin.Context, status int, data templateData) {
	var content bytes.Buffer
	if err := s.templates.ExecuteTemplate(&content, data.ContentTemplate, data); err != nil {
		c.String(http.StatusInternalServerError, "render content failed")
		return
	}
	data.RenderedContent = template.HTML(content.String())

	c.Status(status)
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(c.Writer, "layout", data); err != nil {
		c.String(http.StatusInternalServerError, "render failed")
		return
	}
}

func (s *server) renderContentTemplate(c *gin.Context, status int, data templateData) {
	c.Status(status)
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(c.Writer, data.ContentTemplate, data); err != nil {
		c.String(http.StatusInternalServerError, "render content failed")
		return
	}
}

func toProjectCardViews(projects []store.Project) []projectCardView {
	views := make([]projectCardView, 0, len(projects))
	for _, p := range projects {
		views = append(views, projectCardView{
			Name:          p.Name,
			Slug:          p.Slug,
			GoalPrompt:    p.GoalPrompt,
			UpdatedAtText: p.UpdatedAt.Local().Format("2006-01-02 15:04"),
		})
	}
	return views
}

func toProjectDetailView(p store.Project) *projectDetailView {
	publishedAtText := ""
	if p.PublishedAt != nil {
		publishedAtText = p.PublishedAt.Local().Format("2006-01-02 15:04")
	}
	return &projectDetailView{
		ID:                p.ID,
		Name:              p.Name,
		Slug:              p.Slug,
		GoalPrompt:        p.GoalPrompt,
		DraftSpecJSON:     p.DraftSpecJSON,
		PublishedSpecJSON: p.PublishedSpecJSON,
		PublishedSlug:     p.PublishedSlug,
		ShareSlug:         p.ShareSlug,
		PublishedAtText:   publishedAtText,
		CreatedAtText:     p.CreatedAt.Local().Format("2006-01-02 15:04"),
		UpdatedAtText:     p.UpdatedAt.Local().Format("2006-01-02 15:04"),
	}
}

func toChatMessageViews(messages []store.ChatMessage) []chatMessageView {
	views := make([]chatMessageView, 0, len(messages))
	for _, m := range messages {
		view := chatMessageView{
			Role:          m.Role,
			Content:       m.Content,
			CreatedAtText: m.CreatedAt.Local().Format("15:04:05"),
		}
		switch m.Role {
		case store.ChatRoleUser:
			view.RoleLabel = "User"
			view.RoleClass = "chat-role-user"
		case store.ChatRoleAssistant:
			view.RoleLabel = "AI"
			view.RoleClass = "chat-role-assistant"
		default:
			view.RoleLabel = "System"
			view.RoleClass = "chat-role-system"
		}
		views = append(views, view)
	}
	return views
}

func isHTMXRequest(c *gin.Context) bool {
	return strings.EqualFold(strings.TrimSpace(c.GetHeader("HX-Request")), "true")
}

func buildGenerationClient(cfg config.Config) (generation.Client, string) {
	if strings.TrimSpace(cfg.DeepSeekAPIKey) != "" {
		log.Printf("generation provider selected: provider=DeepSeek app_base_url=%s", strings.TrimSpace(cfg.AppBaseURL))
		client := generation.NewDeepSeekClient(generation.DeepSeekClientConfig{
			APIKey:     cfg.DeepSeekAPIKey,
			AppBaseURL: cfg.AppBaseURL,
		})
		return client, "DeepSeek"
	}
	log.Printf("generation provider selected: provider=Stub reason=missing_deepseek_api_key")
	return generation.NewStubClient(), "Stub"
}

func (s *server) handlePublishedProjectPage(c *gin.Context) {
	s.renderPublicProjectPage(c, http.StatusOK, c.Param("slug"), publicProjectPageOptions{
		routeSlug:  c.Param("slug"),
		kind:       "published",
		mode:       apprender.ModeEditor,
		specSource: "published",
		uiState:    previewUIStateFromRequest(c),
	})
}

func (s *server) handlePublishedProjectPreviewPanel(c *gin.Context) {
	s.renderPublicProjectPreviewPanel(c, http.StatusOK, c.Param("slug"), publicProjectPageOptions{
		routeSlug:  c.Param("slug"),
		kind:       "published",
		mode:       apprender.ModeEditor,
		specSource: "published",
		uiState:    previewUIStateFromRequest(c),
	})
}

func (s *server) handlePublishedProjectPreviewFramePage(c *gin.Context) {
	s.renderPublicProjectPreviewFramePage(c, http.StatusOK, c.Param("slug"), publicProjectPageOptions{
		routeSlug:  c.Param("slug"),
		kind:       "published",
		mode:       apprender.ModeEditor,
		specSource: "published",
		uiState:    previewUIStateFromRequest(c),
	})
}

func (s *server) handleSharedProjectPage(c *gin.Context) {
	s.renderPublicProjectPage(c, http.StatusOK, c.Param("slug"), publicProjectPageOptions{
		routeSlug:  c.Param("slug"),
		kind:       "share",
		mode:       apprender.ModeShareReadonly,
		specSource: "draft",
		uiState:    previewUIStateFromRequest(c),
	})
}

func (s *server) handleSharedProjectPreviewPanel(c *gin.Context) {
	s.renderPublicProjectPreviewPanel(c, http.StatusOK, c.Param("slug"), publicProjectPageOptions{
		routeSlug:  c.Param("slug"),
		kind:       "share",
		mode:       apprender.ModeShareReadonly,
		specSource: "draft",
		uiState:    previewUIStateFromRequest(c),
	})
}

func (s *server) handleSharedProjectPreviewFramePage(c *gin.Context) {
	s.renderPublicProjectPreviewFramePage(c, http.StatusOK, c.Param("slug"), publicProjectPageOptions{
		routeSlug:  c.Param("slug"),
		kind:       "share",
		mode:       apprender.ModeShareReadonly,
		specSource: "draft",
		uiState:    previewUIStateFromRequest(c),
	})
}

func (s *server) handleSharedPreviewWriteForbidden(c *gin.Context) {
	c.String(http.StatusForbidden, "share readonly mode forbids write operations")
}

func (s *server) handlePublishedPreviewCreateRecordSubmit(c *gin.Context) {
	s.handlePublicPreviewCreateRecordSubmit(c, "published")
}

func (s *server) handlePublishedPreviewUpdateRecordSubmit(c *gin.Context) {
	s.handlePublicPreviewUpdateRecordSubmit(c, "published")
}

func (s *server) handlePublishedPreviewDeleteRecordSubmit(c *gin.Context) {
	s.handlePublicPreviewDeleteRecordSubmit(c, "published")
}

func (s *server) handlePublishedPreviewToggleRecordSubmit(c *gin.Context) {
	s.handlePublicPreviewToggleRecordSubmit(c, "published")
}

type publicProjectPageOptions struct {
	routeSlug  string
	kind       string // published/share
	mode       apprender.Mode
	specSource string // draft/published
	uiState    previewUIState
}

type publicPreviewWriteContext struct {
	project   store.Project
	appSpec   specpkg.AppSpec
	uiState   previewUIState
	pageRoute publicProjectPageOptions
}

func (s *server) renderPublicProjectPage(c *gin.Context, status int, routeSlug string, opts publicProjectPageOptions) {
	data, err := s.buildPublicProjectTemplateData(c, routeSlug, opts, "")
	if err != nil {
		s.handleProjectDetailRenderError(c, err)
		return
	}
	if opts.kind == "share" {
		data.ContentTemplate = "shared_project_content"
	} else {
		data.ContentTemplate = "published_project_content"
	}
	s.renderTemplate(c, status, data)
}

func (s *server) renderPublicProjectPreviewPanel(c *gin.Context, status int, routeSlug string, opts publicProjectPageOptions) {
	data, err := s.buildPublicProjectTemplateData(c, routeSlug, opts, "")
	if err != nil {
		s.handleProjectDetailRenderError(c, err)
		return
	}
	data.ContentTemplate = "project_detail_preview_panel"
	s.renderContentTemplate(c, status, data)
}

func (s *server) renderPublicProjectPreviewFramePage(c *gin.Context, status int, routeSlug string, opts publicProjectPageOptions) {
	data, err := s.buildPublicProjectTemplateData(c, routeSlug, opts, "")
	if err != nil {
		s.handleProjectDetailRenderError(c, err)
		return
	}
	data.IsPreviewFramePage = true
	data.ContentTemplate = "project_preview_frame_content"
	s.renderTemplate(c, status, data)
}

func (s *server) buildPublicProjectTemplateData(c *gin.Context, routeSlug string, opts publicProjectPageOptions, previewErr string) (templateData, error) {
	project, err := s.loadPublicProjectByRouteSlug(c.Request.Context(), routeSlug, opts.kind)
	if err != nil {
		return templateData{}, err
	}

	data := templateData{
		Title:                  project.Name + " - mini-atoms",
		CurrentProject:         toProjectDetailView(project),
		ShowChatComposer:       false,
		ShowProjectActions:     false,
		ShowBackToProjects:     false,
		ShowDraftSpecPanel:     opts.kind == "share",
		ShowPublishedSpecPanel: true,
		IsSharePage:            opts.kind == "share",
		IsPublishedPage:        opts.kind == "published",
		PreviewMode:            string(opts.mode),
		PreviewError:           previewErr,
		PreviewPageID:          strings.TrimSpace(opts.uiState.PageID),
	}

	var chatMessages []store.ChatMessage
	if opts.kind == "share" {
		chatMessages, err = s.chatRepo.ListMessagesByProject(c.Request.Context(), project.ID)
		if err != nil {
			return templateData{}, err
		}
		data.ChatMessages = toChatMessageViews(chatMessages)
	}

	switch opts.kind {
	case "share":
		data.PreviewPageBasePath = "/share/" + project.ShareSlug
		data.PreviewPanelPath = "/share/" + project.ShareSlug + "/preview"
		data.PreviewFramePath = "/share/" + project.ShareSlug + "/preview/frame"
		data.PreviewActionBasePath = "/share/" + project.ShareSlug
	case "published":
		data.PreviewPageBasePath = "/p/" + project.PublishedSlug
		data.PreviewPanelPath = "/p/" + project.PublishedSlug + "/preview"
		data.PreviewFramePath = "/p/" + project.PublishedSlug + "/preview/frame"
		data.PreviewActionBasePath = "/p/" + project.PublishedSlug
	}

	view, previewBuildErr := s.buildProjectPreviewAppViewBySource(c.Request.Context(), project, data.PreviewPageID, opts.mode, opts.specSource)
	if previewBuildErr != nil {
		data.PreviewError = previewBuildErr.Error()
	} else {
		data.PreviewApp = view
		if data.PreviewPageID == "" && view != nil {
			data.PreviewPageID = view.CurrentPage.ID
		}
	}

	return data, nil
}

func (s *server) loadPublicProjectByRouteSlug(ctx context.Context, routeSlug, kind string) (store.Project, error) {
	switch kind {
	case "share":
		return s.projectRepo.GetProjectByShareSlug(ctx, routeSlug)
	case "published":
		return s.projectRepo.GetProjectByPublishedSlug(ctx, routeSlug)
	default:
		return store.Project{}, fmt.Errorf("unknown public page kind %q", kind)
	}
}

func (s *server) loadPublicPreviewWriteContext(c *gin.Context, kind string) (publicPreviewWriteContext, error) {
	uiState := previewUIStateFromRequest(c)
	project, err := s.loadPublicProjectByRouteSlug(c.Request.Context(), c.Param("slug"), kind)
	if err != nil {
		return publicPreviewWriteContext{}, err
	}

	specSource := "published"
	mode := apprender.ModeEditor
	switch kind {
	case "published":
		specSource = "published"
		mode = apprender.ModeEditor
	case "share":
		specSource = "draft"
		mode = apprender.ModeShareReadonly
	default:
		return publicPreviewWriteContext{}, fmt.Errorf("unknown public preview kind %q", kind)
	}

	specJSON := strings.TrimSpace(project.PublishedSpecJSON)
	if specSource == "draft" {
		specJSON = strings.TrimSpace(project.DraftSpecJSON)
	}
	if specJSON == "" {
		return publicPreviewWriteContext{}, fmt.Errorf("preview spec is empty")
	}

	var appSpec specpkg.AppSpec
	if err := json.Unmarshal([]byte(specJSON), &appSpec); err != nil {
		return publicPreviewWriteContext{}, fmt.Errorf("parse preview spec_json: %w", err)
	}
	appSpec, err = s.mergeAppSpecWithStoredCollections(c.Request.Context(), project.ID, appSpec)
	if err != nil {
		return publicPreviewWriteContext{}, fmt.Errorf("merge preview schema with stored collections: %w", err)
	}
	if err := specpkg.ValidateAppSpec(appSpec); err != nil {
		return publicPreviewWriteContext{}, fmt.Errorf("invalid preview spec: %w", err)
	}
	if uiState.PageID == "" && len(appSpec.Pages) > 0 {
		uiState.PageID = appSpec.Pages[0].ID
	}

	return publicPreviewWriteContext{
		project: project,
		appSpec: appSpec,
		uiState: uiState,
		pageRoute: publicProjectPageOptions{
			routeSlug:  c.Param("slug"),
			kind:       kind,
			mode:       mode,
			specSource: specSource,
			uiState:    uiState,
		},
	}, nil
}

func (s *server) renderPublicPreviewWriteError(c *gin.Context, routeSlug string, opts publicProjectPageOptions, msg string) {
	if isHTMXRequest(c) {
		data, err := s.buildPublicProjectTemplateData(c, routeSlug, opts, msg)
		if err != nil {
			s.handleProjectDetailRenderError(c, err)
			return
		}
		data.ContentTemplate = "project_detail_preview_panel"
		s.renderContentTemplate(c, http.StatusBadRequest, data)
		return
	}
	s.renderPublicProjectPage(c, http.StatusBadRequest, routeSlug, opts)
}

func (s *server) handlePublicPreviewCreateRecordSubmit(c *gin.Context, kind string) {
	ctx, err := s.loadPublicPreviewWriteContext(c, kind)
	if err != nil {
		s.handleProjectDetailRenderError(c, err)
		return
	}
	if ctx.pageRoute.mode.IsReadOnly() || apprender.ParseMode(c.PostForm("preview_mode")).IsReadOnly() {
		c.String(http.StatusForbidden, "share readonly mode forbids write operations")
		return
	}

	if err := s.collectionRepo.SyncCollectionsFromSpec(c.Request.Context(), ctx.project.ID, ctx.appSpec); err != nil {
		s.renderPublicPreviewWriteError(c, c.Param("slug"), ctx.pageRoute, "同步集合失败："+err.Error())
		return
	}
	collection, err := s.collectionRepo.GetCollectionByProjectAndName(c.Request.Context(), ctx.project.ID, c.Param("collection"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.renderPublicPreviewWriteError(c, c.Param("slug"), ctx.pageRoute, "集合不存在")
			return
		}
		s.handleProjectDetailRenderError(c, err)
		return
	}
	inputs, err := previewRecordInputMapFromRequest(c)
	if err != nil {
		s.renderPublicPreviewWriteError(c, c.Param("slug"), ctx.pageRoute, "读取表单失败")
		return
	}
	if _, err := s.recordRepo.CreateRecord(c.Request.Context(), ctx.project.ID, collection, inputs); err != nil {
		s.renderPublicPreviewWriteError(c, c.Param("slug"), ctx.pageRoute, "创建记录失败："+err.Error())
		return
	}
	if isHTMXRequest(c) {
		s.renderPublicProjectPreviewPanel(c, http.StatusOK, c.Param("slug"), ctx.pageRoute)
		return
	}
	c.Redirect(http.StatusSeeOther, ctx.pageRoute.pageRouteBasePath()+buildPreviewQueryString(ctx.pageRoute.uiState, false))
}

func (s *server) handlePublicPreviewUpdateRecordSubmit(c *gin.Context, kind string) {
	ctx, err := s.loadPublicPreviewWriteContext(c, kind)
	if err != nil {
		s.handleProjectDetailRenderError(c, err)
		return
	}
	if ctx.pageRoute.mode.IsReadOnly() || apprender.ParseMode(c.PostForm("preview_mode")).IsReadOnly() {
		c.String(http.StatusForbidden, "share readonly mode forbids write operations")
		return
	}

	if err := s.collectionRepo.SyncCollectionsFromSpec(c.Request.Context(), ctx.project.ID, ctx.appSpec); err != nil {
		s.renderPublicPreviewWriteError(c, c.Param("slug"), ctx.pageRoute, "同步集合失败："+err.Error())
		return
	}
	collection, err := s.collectionRepo.GetCollectionByProjectAndName(c.Request.Context(), ctx.project.ID, c.Param("collection"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.renderPublicPreviewWriteError(c, c.Param("slug"), ctx.pageRoute, "集合不存在")
			return
		}
		s.handleProjectDetailRenderError(c, err)
		return
	}
	recordID, err := strconv.ParseInt(strings.TrimSpace(c.Param("recordID")), 10, 64)
	if err != nil || recordID <= 0 {
		s.renderPublicPreviewWriteError(c, c.Param("slug"), ctx.pageRoute, "记录 ID 无效")
		return
	}
	inputs, err := previewRecordInputMapFromRequest(c)
	if err != nil {
		ctx.pageRoute.uiState.EditCollection = collection.Name
		ctx.pageRoute.uiState.EditRecordID = recordID
		s.renderPublicPreviewWriteError(c, c.Param("slug"), ctx.pageRoute, "读取表单失败")
		return
	}
	if _, err := s.recordRepo.UpdateRecord(c.Request.Context(), ctx.project.ID, collection, recordID, inputs); err != nil {
		ctx.pageRoute.uiState.EditCollection = collection.Name
		ctx.pageRoute.uiState.EditRecordID = recordID
		s.renderPublicPreviewWriteError(c, c.Param("slug"), ctx.pageRoute, "更新记录失败："+err.Error())
		return
	}
	ctx.pageRoute.uiState.EditCollection = ""
	ctx.pageRoute.uiState.EditRecordID = 0
	if isHTMXRequest(c) {
		s.renderPublicProjectPreviewPanel(c, http.StatusOK, c.Param("slug"), ctx.pageRoute)
		return
	}
	c.Redirect(http.StatusSeeOther, ctx.pageRoute.pageRouteBasePath()+buildPreviewQueryString(ctx.pageRoute.uiState, false))
}

func (s *server) handlePublicPreviewDeleteRecordSubmit(c *gin.Context, kind string) {
	ctx, err := s.loadPublicPreviewWriteContext(c, kind)
	if err != nil {
		s.handleProjectDetailRenderError(c, err)
		return
	}
	if ctx.pageRoute.mode.IsReadOnly() || apprender.ParseMode(c.PostForm("preview_mode")).IsReadOnly() {
		c.String(http.StatusForbidden, "share readonly mode forbids write operations")
		return
	}
	if err := s.collectionRepo.SyncCollectionsFromSpec(c.Request.Context(), ctx.project.ID, ctx.appSpec); err != nil {
		s.renderPublicPreviewWriteError(c, c.Param("slug"), ctx.pageRoute, "同步集合失败："+err.Error())
		return
	}
	collection, err := s.collectionRepo.GetCollectionByProjectAndName(c.Request.Context(), ctx.project.ID, c.Param("collection"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.renderPublicPreviewWriteError(c, c.Param("slug"), ctx.pageRoute, "集合不存在")
			return
		}
		s.handleProjectDetailRenderError(c, err)
		return
	}
	recordID, err := strconv.ParseInt(strings.TrimSpace(c.Param("recordID")), 10, 64)
	if err != nil || recordID <= 0 {
		s.renderPublicPreviewWriteError(c, c.Param("slug"), ctx.pageRoute, "记录 ID 无效")
		return
	}
	if err := s.recordRepo.DeleteRecord(c.Request.Context(), ctx.project.ID, collection.ID, recordID); err != nil {
		s.renderPublicPreviewWriteError(c, c.Param("slug"), ctx.pageRoute, "删除记录失败："+err.Error())
		return
	}
	if ctx.pageRoute.uiState.EditCollection == collection.Name && ctx.pageRoute.uiState.EditRecordID == recordID {
		ctx.pageRoute.uiState.EditCollection = ""
		ctx.pageRoute.uiState.EditRecordID = 0
	}
	if isHTMXRequest(c) {
		s.renderPublicProjectPreviewPanel(c, http.StatusOK, c.Param("slug"), ctx.pageRoute)
		return
	}
	c.Redirect(http.StatusSeeOther, ctx.pageRoute.pageRouteBasePath()+buildPreviewQueryString(ctx.pageRoute.uiState, false))
}

func (s *server) handlePublicPreviewToggleRecordSubmit(c *gin.Context, kind string) {
	ctx, err := s.loadPublicPreviewWriteContext(c, kind)
	if err != nil {
		s.handleProjectDetailRenderError(c, err)
		return
	}
	if ctx.pageRoute.mode.IsReadOnly() || apprender.ParseMode(c.PostForm("preview_mode")).IsReadOnly() {
		c.String(http.StatusForbidden, "share readonly mode forbids write operations")
		return
	}
	if err := s.collectionRepo.SyncCollectionsFromSpec(c.Request.Context(), ctx.project.ID, ctx.appSpec); err != nil {
		s.renderPublicPreviewWriteError(c, c.Param("slug"), ctx.pageRoute, "同步集合失败："+err.Error())
		return
	}
	collection, err := s.collectionRepo.GetCollectionByProjectAndName(c.Request.Context(), ctx.project.ID, c.Param("collection"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.renderPublicPreviewWriteError(c, c.Param("slug"), ctx.pageRoute, "集合不存在")
			return
		}
		s.handleProjectDetailRenderError(c, err)
		return
	}
	recordID, err := strconv.ParseInt(strings.TrimSpace(c.Param("recordID")), 10, 64)
	if err != nil || recordID <= 0 {
		s.renderPublicPreviewWriteError(c, c.Param("slug"), ctx.pageRoute, "记录 ID 无效")
		return
	}
	if _, err := s.recordRepo.ToggleBoolField(c.Request.Context(), ctx.project.ID, collection, recordID, c.Param("field")); err != nil {
		s.renderPublicPreviewWriteError(c, c.Param("slug"), ctx.pageRoute, "切换状态失败："+err.Error())
		return
	}
	if isHTMXRequest(c) {
		s.renderPublicProjectPreviewPanel(c, http.StatusOK, c.Param("slug"), ctx.pageRoute)
		return
	}
	c.Redirect(http.StatusSeeOther, ctx.pageRoute.pageRouteBasePath()+buildPreviewQueryString(ctx.pageRoute.uiState, false))
}

func (opts publicProjectPageOptions) pageRouteBasePath() string {
	switch opts.kind {
	case "published":
		return "/p/" + opts.routeSlugValue()
	case "share":
		return "/share/" + opts.routeSlugValue()
	default:
		return "/"
	}
}

func (opts publicProjectPageOptions) routeSlugValue() string {
	return strings.TrimSpace(opts.routeSlug)
}

func (s *server) loadProjectDraftSpec(ctx context.Context, userID int64, slug, pageID string) (store.Project, specpkg.AppSpec, string, error) {
	project, err := s.projectRepo.GetProjectByUserAndSlug(ctx, userID, slug)
	if err != nil {
		return store.Project{}, specpkg.AppSpec{}, "", err
	}
	if strings.TrimSpace(project.DraftSpecJSON) == "" {
		return store.Project{}, specpkg.AppSpec{}, "", fmt.Errorf("draft spec is empty")
	}

	var appSpec specpkg.AppSpec
	if err := json.Unmarshal([]byte(project.DraftSpecJSON), &appSpec); err != nil {
		return store.Project{}, specpkg.AppSpec{}, "", fmt.Errorf("parse draft spec_json: %w", err)
	}
	appSpec, err = s.mergeAppSpecWithStoredCollections(ctx, project.ID, appSpec)
	if err != nil {
		return store.Project{}, specpkg.AppSpec{}, "", fmt.Errorf("merge draft schema with stored collections: %w", err)
	}
	if err := specpkg.ValidateAppSpec(appSpec); err != nil {
		return store.Project{}, specpkg.AppSpec{}, "", fmt.Errorf("invalid draft spec: %w", err)
	}

	selectedPageID := strings.TrimSpace(pageID)
	if selectedPageID == "" && len(appSpec.Pages) > 0 {
		selectedPageID = appSpec.Pages[0].ID
	}
	return project, appSpec, selectedPageID, nil
}

func (s *server) buildDraftPreviewAppView(ctx context.Context, project store.Project, selectedPageID string, mode apprender.Mode) (*apprender.AppView, error) {
	return s.buildProjectPreviewAppViewBySource(ctx, project, selectedPageID, mode, "draft")
}

func (s *server) buildProjectPreviewAppViewBySource(ctx context.Context, project store.Project, selectedPageID string, mode apprender.Mode, specSource string) (*apprender.AppView, error) {
	specJSON := strings.TrimSpace(project.DraftSpecJSON)
	if specSource == "published" {
		specJSON = strings.TrimSpace(project.PublishedSpecJSON)
	}
	if specJSON == "" {
		if specSource == "published" {
			return nil, fmt.Errorf("预览已发布应用失败：尚未发布")
		}
		return nil, nil
	}
	return s.buildPreviewAppViewFromSpecJSON(ctx, project, specJSON, selectedPageID, mode)
}

func (s *server) buildPreviewAppViewFromSpecJSON(ctx context.Context, project store.Project, specJSON, selectedPageID string, mode apprender.Mode) (*apprender.AppView, error) {
	if strings.TrimSpace(project.DraftSpecJSON) == "" {
		// keep behavior compatible for empty draft when caller passes empty specJSON
		if strings.TrimSpace(specJSON) == "" {
			return nil, nil
		}
	}

	var appSpec specpkg.AppSpec
	if err := json.Unmarshal([]byte(specJSON), &appSpec); err != nil {
		return nil, fmt.Errorf("预览草稿失败：Spec JSON 解析失败：%w", err)
	}
	mergedSpec, err := s.mergeAppSpecWithStoredCollections(ctx, project.ID, appSpec)
	if err != nil {
		return nil, fmt.Errorf("预览草稿失败：合并历史集合 Schema 失败：%w", err)
	}
	appSpec = mergedSpec
	if err := specpkg.ValidateAppSpec(appSpec); err != nil {
		return nil, fmt.Errorf("预览草稿失败：Spec 校验失败：%w", err)
	}
	if err := s.collectionRepo.SyncCollectionsFromSpec(ctx, project.ID, appSpec); err != nil {
		return nil, fmt.Errorf("预览草稿失败：同步集合失败：%w", err)
	}

	collectionsData := make(map[string]apprender.CollectionData, len(appSpec.Collections))
	for _, c := range appSpec.Collections {
		row, err := s.collectionRepo.GetCollectionByProjectAndName(ctx, project.ID, c.Name)
		if err != nil {
			return nil, fmt.Errorf("预览草稿失败：读取集合 %q 失败：%w", c.Name, err)
		}
		records, err := s.recordRepo.ListRecordsByCollection(ctx, project.ID, row.ID)
		if err != nil {
			return nil, fmt.Errorf("预览草稿失败：读取记录 %q 失败：%w", c.Name, err)
		}
		renderRecords := make([]apprender.Record, 0, len(records))
		for _, rec := range records {
			renderRecords = append(renderRecords, apprender.Record{
				ID:   rec.ID,
				Data: rec.Data,
			})
		}
		collectionsData[c.Name] = apprender.CollectionData{
			Schema:  c,
			Records: renderRecords,
		}
	}

	view, err := apprender.RenderApp(apprender.RenderInput{
		Spec:           appSpec,
		Mode:           mode,
		SelectedPageID: selectedPageID,
		Collections:    collectionsData,
	})
	if err != nil {
		return nil, fmt.Errorf("预览草稿失败：%w", err)
	}
	return &view, nil
}

func (s *server) mergeAppSpecWithStoredCollections(ctx context.Context, projectID int64, appSpec specpkg.AppSpec) (specpkg.AppSpec, error) {
	if projectID == 0 {
		return appSpec, nil
	}

	rows, err := s.collectionRepo.ListCollectionsByProject(ctx, projectID)
	if err != nil {
		return specpkg.AppSpec{}, fmt.Errorf("list stored collections: %w", err)
	}
	if len(rows) == 0 {
		return appSpec, nil
	}

	out := appSpec
	collectionIdx := make(map[string]int, len(out.Collections))
	for i, c := range out.Collections {
		collectionIdx[c.Name] = i
	}

	for _, row := range rows {
		var stored specpkg.CollectionSpec
		if err := json.Unmarshal([]byte(row.SchemaJSON), &stored); err != nil {
			return specpkg.AppSpec{}, fmt.Errorf("parse stored collection %q schema_json: %w", row.Name, err)
		}
		idx, ok := collectionIdx[stored.Name]
		if !ok {
			out.Collections = append(out.Collections, stored)
			collectionIdx[stored.Name] = len(out.Collections) - 1
			continue
		}

		merged := out.Collections[idx]
		fieldNames := make(map[string]struct{}, len(merged.Fields))
		for _, f := range merged.Fields {
			fieldNames[f.Name] = struct{}{}
		}
		for _, f := range stored.Fields {
			if _, exists := fieldNames[f.Name]; exists {
				continue
			}
			merged.Fields = append(merged.Fields, f)
			fieldNames[f.Name] = struct{}{}
		}
		out.Collections[idx] = merged
	}

	return out, nil
}

func previewUIStateFromRequest(c *gin.Context) previewUIState {
	state := previewUIState{
		PageID:         strings.TrimSpace(firstNonEmpty(c.PostForm("page_id"), c.Query("page"))),
		EditCollection: strings.TrimSpace(firstNonEmpty(c.PostForm("edit_collection"), c.Query("edit_collection"))),
	}
	editRecordRaw := strings.TrimSpace(firstNonEmpty(c.PostForm("edit_record"), c.Query("edit_record")))
	if editRecordRaw != "" {
		if id, err := strconv.ParseInt(editRecordRaw, 10, 64); err == nil && id > 0 {
			state.EditRecordID = id
		}
	}
	return state
}

func previewRecordInputMapFromRequest(c *gin.Context) (map[string]string, error) {
	if err := c.Request.ParseForm(); err != nil {
		return nil, err
	}
	inputs := make(map[string]string)
	for key, vals := range c.Request.PostForm {
		switch key {
		case "page_id", "preview_mode", "edit_collection", "edit_record":
			continue
		}
		if len(vals) == 0 {
			continue
		}
		inputs[key] = vals[len(vals)-1]
	}
	return inputs, nil
}

func buildPreviewQueryString(state previewUIState, includeLeadingQuestion bool) string {
	values := url.Values{}
	if strings.TrimSpace(state.PageID) != "" {
		values.Set("page", strings.TrimSpace(state.PageID))
	}
	if strings.TrimSpace(state.EditCollection) != "" && state.EditRecordID > 0 {
		values.Set("edit_collection", strings.TrimSpace(state.EditCollection))
		values.Set("edit_record", strconv.FormatInt(state.EditRecordID, 10))
	}
	qs := values.Encode()
	if qs == "" {
		return ""
	}
	if includeLeadingQuestion {
		return "?" + qs
	}
	return "?" + qs
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func (s *server) loadRecordEditValues(ctx context.Context, projectID int64, collectionName string, recordID int64) (map[string]string, error) {
	collection, err := s.collectionRepo.GetCollectionByProjectAndName(ctx, projectID, collectionName)
	if err != nil {
		return nil, err
	}
	record, err := s.recordRepo.GetRecordByID(ctx, projectID, collection.ID, recordID)
	if err != nil {
		return nil, err
	}
	var schema specpkg.CollectionSpec
	if err := json.Unmarshal([]byte(collection.SchemaJSON), &schema); err != nil {
		return nil, fmt.Errorf("parse collection schema_json: %w", err)
	}
	out := make(map[string]string, len(schema.Fields))
	for _, f := range schema.Fields {
		out[f.Name] = recordValueForFormField(f, record.Data[f.Name])
	}
	return out, nil
}

func recordValueForFormField(field specpkg.FieldSpec, v any) string {
	if v == nil {
		return ""
	}
	switch field.Type {
	case specpkg.FieldTypeBool:
		switch x := v.(type) {
		case bool:
			if x {
				return "1"
			}
			return "0"
		case float64:
			if x != 0 {
				return "1"
			}
			return "0"
		case string:
			if strings.EqualFold(strings.TrimSpace(x), "true") || strings.TrimSpace(x) == "1" {
				return "1"
			}
			return "0"
		default:
			return "0"
		}
	case specpkg.FieldTypeInt:
		switch x := v.(type) {
		case float64:
			return strconv.FormatInt(int64(x), 10)
		case int64:
			return strconv.FormatInt(x, 10)
		case string:
			return strings.TrimSpace(x)
		}
	case specpkg.FieldTypeReal:
		switch x := v.(type) {
		case float64:
			return strconv.FormatFloat(x, 'f', -1, 64)
		case string:
			return strings.TrimSpace(x)
		}
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func publishedPagePath(project store.Project) string {
	if strings.TrimSpace(project.PublishedSlug) == "" {
		return ""
	}
	return "/p/" + project.PublishedSlug
}

func sharePagePath(project store.Project) string {
	if strings.TrimSpace(project.ShareSlug) == "" {
		return ""
	}
	return "/share/" + project.ShareSlug
}

func urlQueryEscape(v string) string {
	return url.QueryEscape(strings.TrimSpace(v))
}
