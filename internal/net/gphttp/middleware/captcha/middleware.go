package captcha

import (
	"net/http"
	"text/template"

	"github.com/rs/zerolog/log"
	"github.com/yusing/go-proxy/internal/auth"
	"github.com/yusing/go-proxy/internal/net/gphttp"

	_ "embed"
)

const cookieName = "godoxy_captcha_session"

//go:embed captcha.html
var captchaPageHTML string
var captchaPage = template.Must(template.New("captcha").Parse(captchaPageHTML))

func PreRequest(p Provider, w http.ResponseWriter, r *http.Request) (proceed bool) {
	// check session
	sessionID, err := r.Cookie(cookieName)
	if err == nil {
		session, ok := CaptchaSessions.Load(sessionID.Value)
		if ok {
			if session.expired() {
				CaptchaSessions.Delete(sessionID.Value)
			} else {
				return true
			}
		}
	}

	if !gphttp.GetAccept(r.Header).AcceptHTML() {
		gphttp.Forbidden(w, "Captcha is required")
		return false
	}

	if r.Method == http.MethodPost {
		err := p.Verify(r)
		if err == nil {
			session := newCaptchaSession(p)
			CaptchaSessions.Store(session.ID, session)
			auth.SetTokenCookie(w, r, cookieName, session.ID, p.SessionExpiry())
			http.Redirect(w, r, r.URL.Path, http.StatusFound)
			return false
		}
		gphttp.Unauthorized(w, err.Error())
		return false
	}

	// captcha challenge
	err = captchaPage.Execute(w, map[string]any{
		"ScriptHTML": p.ScriptHTML(),
		"FormHTML":   p.FormHTML(),
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to execute captcha page")
	}
	return false
}
