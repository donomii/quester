package main

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newTestApp(t *testing.T) (*App, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(store, "/quester/")
	router := gin.New()
	app.Register(router)
	return app, router
}

func TestSummaryEscapesUserContent(t *testing.T) {
	_, router := newTestApp(t)
	form := url.Values{
		"q":       {rootPath},
		"title":   {`<script>alert("x")</script>`},
		"content": {`<b>bold</b>`},
	}
	postForm(t, router, "/quester/addWaypoint", form)

	resp := performRequest(router, http.MethodGet, "/quester/summary", nil)
	defer resp.Body.Close()
	body := readBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	if strings.Contains(body, `<script>alert("x")</script>`) || strings.Contains(body, `<b>bold</b>`) {
		t.Fatalf("response contains unescaped user HTML: %s", body)
	}
	if !strings.Contains(body, `&lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt;`) {
		t.Fatalf("response did not contain escaped title: %s", body)
	}
}

func TestMutationsUsePostAndBadPathsDoNotPanic(t *testing.T) {
	_, router := newTestApp(t)

	getToggle := performRequest(router, http.MethodGet, "/quester/toggle?path="+rootPath, nil)
	getToggle.Body.Close()
	if getToggle.StatusCode != http.StatusNotFound {
		t.Fatalf("GET toggle status = %d, want 404", getToggle.StatusCode)
	}

	deleteMissing := postForm(t, router, "/quester/deleteWaypoint", url.Values{"q": {rootPath + "/missing"}})
	if deleteMissing.StatusCode != http.StatusNotFound {
		body := readBody(t, deleteMissing)
		t.Fatalf("delete missing status = %d, body = %s", deleteMissing.StatusCode, body)
	}
	deleteMissing.Body.Close()
}

func TestMutationRejectsCrossOriginPost(t *testing.T) {
	app, router := newTestApp(t)
	form := url.Values{
		"q":     {rootPath},
		"title": {"Blocked"},
	}
	req := httptest.NewRequest(http.MethodPost, "/quester/addWaypoint", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://example.invalid")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	resp := rec.Result()
	resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin mutation status = %d, want 403", resp.StatusCode)
	}
	root, err := app.store.Load(defaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	if len(root.SubTasks) != 0 {
		t.Fatalf("cross-origin mutation added %d tasks, want 0", len(root.SubTasks))
	}
}

func TestMutationAllowsSameOriginPost(t *testing.T) {
	app, router := newTestApp(t)
	form := url.Values{
		"q":     {rootPath},
		"title": {"Allowed"},
	}
	req := httptest.NewRequest(http.MethodPost, "http://quester.local/quester/addWaypoint", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://quester.local")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	resp := rec.Result()
	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("same-origin mutation status = %d, want 303", resp.StatusCode)
	}
	root, err := app.store.Load(defaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	if len(root.SubTasks) != 1 || root.SubTasks[0].Name != "Allowed" {
		t.Fatalf("same-origin mutation tasks = %#v", root.SubTasks)
	}
}

func TestAddToggleDeleteFlow(t *testing.T) {
	app, router := newTestApp(t)
	postForm(t, router, "/quester/addWaypoint", url.Values{
		"q":     {rootPath},
		"title": {"First"},
	})

	root, err := app.store.Load(defaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	if len(root.SubTasks) != 1 {
		t.Fatalf("subtasks = %d, want 1", len(root.SubTasks))
	}
	path := rootPath + "/" + root.SubTasks[0].Id

	postForm(t, router, "/quester/toggle", url.Values{"path": {path}, "next": {"/quester/summary"}})
	root, err = app.store.Load(defaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	if !root.SubTasks[0].Checked {
		t.Fatal("task was not toggled checked")
	}

	postForm(t, router, "/quester/deleteWaypoint", url.Values{"q": {path}})
	root, err = app.store.Load(defaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	if !root.SubTasks[0].Deleted {
		t.Fatal("task was not marked deleted")
	}
}

func TestEditDownloadAndRestoreFlow(t *testing.T) {
	app, router := newTestApp(t)
	postForm(t, router, "/quester/addWaypoint", url.Values{
		"q":     {rootPath},
		"title": {"Original"},
	})

	root, err := app.store.Load(defaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	path := rootPath + "/" + root.SubTasks[0].Id

	editResp := postForm(t, router, "/quester/editWaypoint", url.Values{
		"q":       {path},
		"title":   {"Updated"},
		"content": {"Edited notes"},
	})
	editResp.Body.Close()
	if editResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("edit status = %d, want 303", editResp.StatusCode)
	}

	download := performRequest(router, http.MethodGet, "/quester/downloadAll", nil)
	downloadBody := readBody(t, download)
	download.Body.Close()
	if download.StatusCode != http.StatusOK {
		t.Fatalf("download status = %d, body = %s", download.StatusCode, downloadBody)
	}
	if got := download.Header.Get("Content-Disposition"); got != `attachment; filename="tasks.json"` {
		t.Fatalf("Content-Disposition = %q", got)
	}
	if !strings.Contains(downloadBody, `"Name": "Updated"`) || !strings.Contains(downloadBody, `"Text": "Edited notes"`) {
		t.Fatalf("download did not include edited task: %s", downloadBody)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("content", "tasks.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(`{"Name":"Restored","SubTasks":[{"Id":"restored-child","Name":"Restored child"}]}`)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/quester/restoreAll", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	restore := rec.Result()
	restore.Body.Close()
	if restore.StatusCode != http.StatusSeeOther {
		t.Fatalf("restore status = %d, want 303", restore.StatusCode)
	}

	restoredRoot, err := app.store.Load(defaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	if restoredRoot.Name != "Restored" {
		t.Fatalf("restored root name = %q, want Restored", restoredRoot.Name)
	}
	if got := FindTask(rootPath+"/restored-child", restoredRoot); got == nil || got.Name != "Restored child" {
		t.Fatalf("restored child = %#v, want Restored child", got)
	}
}

func TestRootRouteRedirectsToSummary(t *testing.T) {
	_, router := newTestApp(t)

	resp := performRequest(router, http.MethodGet, "/quester/", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); got != "/quester/summary" {
		t.Fatalf("Location = %q, want /quester/summary", got)
	}
}

func TestRootPrefixCanRegister(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(store, "/")
	router := gin.New()
	app.Register(router)

	resp := performRequest(router, http.MethodGet, "/", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); got != "/summary" {
		t.Fatalf("Location = %q, want /summary", got)
	}
}

func postForm(t *testing.T, router http.Handler, target string, form url.Values) *http.Response {
	t.Helper()
	return performRequest(router, http.MethodPost, target, strings.NewReader(form.Encode()), "application/x-www-form-urlencoded")
}

func performRequest(router http.Handler, method, target string, body io.Reader, contentType ...string) *http.Response {
	req := httptest.NewRequest(method, target, body)
	if len(contentType) > 0 {
		req.Header.Set("Content-Type", contentType[0])
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec.Result()
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
