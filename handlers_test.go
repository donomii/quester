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

func TestAttachAndServeDocumentFlow(t *testing.T) {
	app, router := newTestApp(t)
	postForm(t, router, "/quester/addWaypoint", url.Values{"q": {rootPath}, "title": {"Doc task"}})

	root, err := app.store.Load(defaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	path := rootPath + "/" + root.SubTasks[0].Id

	attach := postMultipart(t, router, "/quester/attachDocument", url.Values{"q": {path}}, []testUpload{
		{"document", "notes.txt", "first version"},
	})
	attach.Body.Close()
	if attach.StatusCode != http.StatusSeeOther {
		t.Fatalf("attach status = %d, want 303", attach.StatusCode)
	}

	root, err = app.store.Load(defaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	attachments := root.SubTasks[0].Attachments
	if len(attachments) != 1 {
		t.Fatalf("attachments = %#v, want 1", attachments)
	}
	record := attachments[0]
	if record.Name != "notes.txt" || record.Size != int64(len("first version")) || !isBlobRef(record.Blob) {
		t.Fatalf("attachment record = %#v", record)
	}

	doc := performRequest(router, http.MethodGet, "/quester/document?q="+url.QueryEscape(path)+"&doc="+url.QueryEscape(record.Id), nil)
	body := readBody(t, doc)
	doc.Body.Close()
	if doc.StatusCode != http.StatusOK || body != "first version" {
		t.Fatalf("document status = %d body = %q", doc.StatusCode, body)
	}
	if got := doc.Header.Get("Content-Disposition"); !strings.HasPrefix(got, "inline") || !strings.Contains(got, "notes.txt") {
		t.Fatalf("Content-Disposition = %q", got)
	}
	if got := doc.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/plain") {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := doc.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}

	detail := performRequest(router, http.MethodGet, "/quester/detailed?q="+url.QueryEscape(path), nil)
	detailBody := readBody(t, detail)
	detail.Body.Close()
	if !strings.Contains(detailBody, "notes.txt") || !strings.Contains(detailBody, "Documents here") {
		t.Fatalf("detail page missing attachment: %s", detailBody)
	}
	if !strings.Contains(detailBody, record.Blob[:8]) {
		t.Fatalf("detail page missing content ref %s: %s", record.Blob[:8], detailBody)
	}
	if !strings.Contains(detailBody, "Documents in effect: 1") {
		t.Fatalf("detail page missing in-effect line: %s", detailBody)
	}
}

func TestAddWaypointCarriesDocument(t *testing.T) {
	app, router := newTestApp(t)
	resp := postMultipart(t, router, "/quester/addWaypoint", url.Values{
		"q":     {rootPath},
		"title": {"Reply with doc"},
	}, []testUpload{{"document", "spec.md", "updated spec"}})
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("add status = %d, want 303", resp.StatusCode)
	}

	root, err := app.store.Load(defaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	if len(root.SubTasks) != 1 || len(root.SubTasks[0].Attachments) != 1 {
		t.Fatalf("tasks = %#v", root.SubTasks)
	}
	if got := root.SubTasks[0].Attachments[0].Name; got != "spec.md" {
		t.Fatalf("attachment name = %q, want spec.md", got)
	}
}

func TestDocumentBlocksActiveContentAndUnknownIDs(t *testing.T) {
	app, router := newTestApp(t)
	resp := postMultipart(t, router, "/quester/addWaypoint", url.Values{
		"q":     {rootPath},
		"title": {"Evil"},
	}, []testUpload{{"document", "evil.html", "<script>alert(1)</script>"}})
	resp.Body.Close()

	root, err := app.store.Load(defaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	path := rootPath + "/" + root.SubTasks[0].Id
	record := root.SubTasks[0].Attachments[0]

	doc := performRequest(router, http.MethodGet, "/quester/document?q="+url.QueryEscape(path)+"&doc="+url.QueryEscape(record.Id), nil)
	doc.Body.Close()
	if got := doc.Header.Get("Content-Disposition"); !strings.HasPrefix(got, "attachment") {
		t.Fatalf("html Content-Disposition = %q, want attachment", got)
	}

	missing := performRequest(router, http.MethodGet, "/quester/document?q="+url.QueryEscape(path)+"&doc=../../etc/passwd", nil)
	missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("missing doc status = %d, want 404", missing.StatusCode)
	}

	empty := postMultipart(t, router, "/quester/attachDocument", url.Values{"q": {path}}, nil)
	empty.Body.Close()
	if empty.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty attach status = %d, want 400", empty.StatusCode)
	}
}

func TestDocumentContentTypeChoosesSafeDisposition(t *testing.T) {
	cases := []struct {
		name            string
		wantTypePrefix  string
		wantDisposition string
	}{
		{"notes.md", "text/markdown", "inline"},
		{"notes.txt", "text/plain", "inline"},
		{"photo.png", "image/png", "inline"},
		{"page.html", "text/html", "attachment"},
		{"vector.svg", "image/svg+xml", "attachment"},
		{"blob.xyz-unknown", "application/octet-stream", "attachment"},
	}
	for _, tc := range cases {
		gotType, gotDisposition := documentContentType(tc.name)
		if !strings.HasPrefix(gotType, tc.wantTypePrefix) || gotDisposition != tc.wantDisposition {
			t.Fatalf("documentContentType(%q) = %q, %q, want %s*, %s",
				tc.name, gotType, gotDisposition, tc.wantTypePrefix, tc.wantDisposition)
		}
	}
}

type testUpload struct {
	field   string
	name    string
	content string
}

func postMultipart(t *testing.T, router http.Handler, target string, fields url.Values, files []testUpload) *http.Response {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, values := range fields {
		for _, value := range values {
			if err := writer.WriteField(key, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, file := range files {
		part, err := writer.CreateFormFile(file.field, file.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte(file.content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return performRequest(router, http.MethodPost, target, &body, writer.FormDataContentType())
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
