// Package onelink — одноразовые ссылки с редиректом на внешний ресурс.
// Ссылка сгорает в две фазы, чтобы фоновое/предварительное открытие в WebView
// (превью, instant app, URL-сканер) не сожгло её раньше времени:
//
//	GET/POST {prefix}/{unq}/		— «подготовка»: HTML без пометки использованной
//	GET/POST {prefix}/{unq}/go_to	— «факт перехода»: атомарная пометка + редирект
//
// Подключается к чужому серверу как обычный http.Handler:
//
//	mux.Handle("/redirect/<unique_key>/", onelink.New(abstractStore, key))
package onelink

import (
	"html/template"
	"net/http"
	"path"
)

type abstractStore interface {
	GetRedirectParam(string) (string, string, map[string]string, bool)
	DeleteRedirectParam(string)
}

type Handler struct {
	store   abstractStore
	keyName string
}

func New(store abstractStore, key string) http.Handler {
	if store == nil {
		panic("onelink: store is nil")
	}
	return &Handler{store: store, keyName: key}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodPost: // pass
	default:
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	key := r.PathValue(h.keyName)
	if key == "" {
		http.NotFound(w, r)
		return
	}
	method, url, param, found := h.store.GetRedirectParam(key)
	if !found {
		http.NotFound(w, r)
		return
	}
	if path.Base(r.URL.Path) == "go_to" {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		defer h.store.DeleteRedirectParam(key)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch method {
		case http.MethodGet:
			postTmpl.Execute(w, map[string]string{"RedirectURL": url})
		case http.MethodPost:
			getTmpl.Execute(w, map[string]any{"Action": url, "Params": param})
		}
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	getTmpl.Execute(w, map[string]string{"RedirectURL": path.Join(r.RequestURI, "go_to")})
}

var getTmpl = template.Must(template.New("get").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"></head>
<noscript><a href="{{.RedirectURL}}">continue</a></noscript>
<script>window.location.replace("{{.RedirectURL}}");</script>
</body></html>`))

var postTmpl = template.Must(template.New("post").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"></head>
<body><form id="f" method="post" action="{{.Action}}">
{{range .Params}}<input type="hidden" name="{{.Key}}" value="{{.Value}}">
{{end}}</form>
<noscript><button form="f" type="submit">continue</button></noscript>
<script>document.getElementById("f").submit();</script>
</body></html>`))
