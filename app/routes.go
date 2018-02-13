package app

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/nlopes/slack"
)

func (app *App) SetupRouter() *mux.Router {
	router := mux.NewRouter()
	router.HandleFunc("/", app.HandleIndex).Methods("GET")
	router.HandleFunc("/favicon.ico", app.HandleFavicon).Methods("GET")
	router.HandleFunc("/success", app.HandleAuthSuccess).Methods("GET")
	router.HandleFunc("/oauth/callback", app.HandleOAuthCallback).Methods("GET")
	router.HandleFunc("/oauth/authenticate/{state}", app.HandleAuthenticate).Methods("GET")
	router.HandleFunc("/hooks/slash", app.HandleSlashCommand).Methods("POST")
	router.HandleFunc("/hooks/interactive", app.HandleActionCallback).Methods("POST")
	return router
}

func (app *App) HandleIndex(w http.ResponseWriter, r *http.Request) {
	app.handleAsset("index.html", w, r)
}

func (app *App) HandleAuthSuccess(w http.ResponseWriter, r *http.Request) {
	app.handleAsset("success.html", w, r)
}

func (app *App) HandleFavicon(w http.ResponseWriter, r *http.Request) {
	app.handleAsset("favicon.ico", w, r)
}

func (app *App) handleAsset(filename string, w http.ResponseWriter, r *http.Request) {
	data, err := Asset("assets/" + filename)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	} else {
		w.Write(data)
	}
}

func (app *App) HandleAuthenticate(w http.ResponseWriter, r *http.Request) {
	app.ReconnectRedisIfNeeeded()
	vars := mux.Vars(r)
	state := vars["state"]
	ctx := app.CreateContext(r)
	if userID := ctx.GetUserIDForState(state); userID == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	config := ctx.GetOAuth2Config()
	url := config.AuthCodeURL(state)
	http.Redirect(w, r, url, http.StatusSeeOther)
}

func (app *App) HandleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	app.ReconnectRedisIfNeeeded()
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	ctx := app.CreateContext(r)
	token, err := ctx.GetAccessToken(code, state)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	ctx.UserID = ctx.GetUserIDForState(state)
	ctx.SetAccessToken(token)
	ctx.DeleteState(state)
	http.Redirect(w, r, "/success", http.StatusFound)
}

func (app *App) HandleSlashCommand(w http.ResponseWriter, r *http.Request) {
	app.ReconnectRedisIfNeeeded()
	s, err := slack.SlashCommandParse(r)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if !s.ValidateToken(app.SlackVerificationToken) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	ctx := app.CreateContext(r)
	ctx.UserID = s.UserID

	params, err := ctx.GetSlackMessage(s.Text)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	b, err := json.Marshal(params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}

func (app *App) HandleActionCallback(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	payload := r.PostForm.Get("payload")

	var data slack.AttachmentActionCallback
	if err := json.Unmarshal([]byte(payload), &data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if data.Token != app.SlackVerificationToken {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	text := ""
	switch data.Actions[0].Name {
	case ActionTypeLeave:
		text = "退社しました"
	case ActionTypeRest:
		text = "休憩を開始しました"
	case ActionTypeUnrest:
		text = "休憩を終了しました"
	case ActionTypeAttend:
		text = "出社しました"
	}

	params := &slack.Msg{
		ResponseType:    "in_channel",
		ReplaceOriginal: true,
		Text:            text,
	}

	b, err := json.Marshal(params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}
