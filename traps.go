package main

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type trap struct {
	name    string
	match   func(path string) bool
	respond func(w http.ResponseWriter, r *http.Request, f fakes)
}

func exact(paths ...string) func(string) bool {
	set := map[string]bool{}
	for _, p := range paths {
		set[p] = true
	}
	return func(path string) bool { return set[strings.ToLower(strings.TrimSuffix(path, "/"))] }
}

func prefix(prefixes ...string) func(string) bool {
	return func(path string) bool {
		lower := strings.ToLower(path)
		for _, p := range prefixes {
			if strings.HasPrefix(lower, p) {
				return true
			}
		}
		return false
	}
}

func segment(names ...string) func(string) bool {
	return func(path string) bool {
		for _, s := range strings.Split(strings.ToLower(path), "/") {
			for _, n := range names {
				if s == n || strings.HasPrefix(s, n+".") {
					return true
				}
			}
		}
		return false
	}
}

var traps = []trap{
	{"portal-login", exact("", "/login", "/admin", "/admin/login", "/index.php", "/index.html"), portalLogin},
	{"robots", exact("/robots.txt"), robots},
	{"dotenv", segment(".env"), dotenv},
	{"git", prefix("/.git/"), gitFiles},
	{"aws-credentials", segment(".aws"), awsCredentials},
	{"wp-login", exact("/wp-login.php"), wpLogin},
	{"wp-admin", prefix("/wp-admin"), redirect("/wp-login.php?redirect_to=%2Fwp-admin%2F&reauth=1")},
	{"xmlrpc", exact("/xmlrpc.php"), xmlrpc},
	{"wp-users", prefix("/wp-json/wp/v2/users"), wpUsers},
	{"phpmyadmin", prefix("/phpmyadmin", "/pma", "/myadmin", "/mysql/admin", "/dbadmin"), phpMyAdmin},
	{"actuator", prefix("/actuator"), actuator},
	{"config-json", exact("/config.json", "/api/config", "/config/app.json"), configJSON},
	{"phpunit", prefix("/vendor/phpunit/"), empty(http.StatusOK)},
	{"cgi-bin", prefix("/cgi-bin/"), empty(http.StatusForbidden)},
	{"server-status", exact("/server-status", "/server-info"), empty(http.StatusForbidden)},
	{"router-exploit", prefix("/hnap1", "/boaform/", "/gponform/", "/goform/", "/setup.cgi", "/shell"), empty(http.StatusOK)},
	{"backup", segment("backup", "backups", "db", "dump", "database"), empty(http.StatusForbidden)},
	{"api", prefix("/api/"), apiUnauthorized},
}

var notFound = trap{"not-found", nil, notFoundPage}

func route(r *http.Request) trap {
	for _, t := range traps {
		if t.match(r.URL.Path) {
			return t
		}
	}
	return notFound
}

func html(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	w.Header().Set("X-Powered-By", "PHP/8.2.12")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func text(w http.ResponseWriter, status int, contentType, body string) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func empty(status int) func(http.ResponseWriter, *http.Request, fakes) {
	return func(w http.ResponseWriter, _ *http.Request, _ fakes) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		w.WriteHeader(status)
	}
}

func redirect(to string) func(http.ResponseWriter, *http.Request, fakes) {
	return func(w http.ResponseWriter, r *http.Request, _ fakes) {
		http.Redirect(w, r, to, http.StatusFound)
	}
}

const loginPage = `<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>%s</title>
<style>body{font-family:sans-serif;background:#f3f4f6;display:flex;justify-content:center;padding-top:10vh}form{background:#fff;padding:2rem;border-radius:8px;width:320px;box-shadow:0 1px 3px rgba(0,0,0,.2)}input{width:100%%;margin:.4rem 0 1rem;padding:.5rem;box-sizing:border-box}button{width:100%%;padding:.6rem}.err{color:#b91c1c}</style>
</head>
<body><form method="post" action="%s"><h2>%s</h2>%s
<label>Username</label><input name="%s" autocomplete="username">
<label>Password</label><input name="%s" type="password" autocomplete="current-password">
<button type="submit">Sign in</button></form></body></html>`

func loginForm(w http.ResponseWriter, r *http.Request, title, action, user, pass string) {
	msg := ""
	status := http.StatusOK
	if r.Method == http.MethodPost {
		msg = `<p class="err">Invalid username or password.</p>`
		status = http.StatusUnauthorized
	}
	html(w, status, fmt.Sprintf(loginPage, title, action, title, msg, user, pass))
}

func portalLogin(w http.ResponseWriter, r *http.Request, _ fakes) {
	loginForm(w, r, "Admin Portal", "/login", "username", "password")
}

func wpLogin(w http.ResponseWriter, r *http.Request, _ fakes) {
	loginForm(w, r, "Log In &lsaquo; Admin Portal &#8212; WordPress", "/wp-login.php", "log", "pwd")
}

func phpMyAdmin(w http.ResponseWriter, r *http.Request, _ fakes) {
	loginForm(w, r, "phpMyAdmin", r.URL.Path, "pma_username", "pma_password")
}

func robots(w http.ResponseWriter, _ *http.Request, _ fakes) {
	text(w, http.StatusOK, "text/plain; charset=utf-8", "User-agent: *\nDisallow: /admin/\nDisallow: /backup/\nDisallow: /api/\nDisallow: /phpmyadmin/\n")
}

func dotenv(w http.ResponseWriter, _ *http.Request, f fakes) {
	text(w, http.StatusOK, "text/plain; charset=utf-8", fmt.Sprintf(`APP_NAME="Admin Portal"
APP_ENV=production
APP_KEY=%s
APP_DEBUG=false
APP_URL=https://admin-portal.internal

DB_CONNECTION=mysql
DB_HOST=10.20.0.15
DB_PORT=3306
DB_DATABASE=portal
DB_USERNAME=portal
DB_PASSWORD=%s

MAIL_MAILER=smtp
MAIL_HOST=smtp.internal
MAIL_PORT=587
MAIL_USERNAME=noreply@admin-portal.internal
MAIL_PASSWORD=%s

AWS_ACCESS_KEY_ID=%s
AWS_SECRET_ACCESS_KEY=%s
AWS_DEFAULT_REGION=eu-west-3

JWT_SECRET=%s
`, f.appKey, f.dbPassword, f.mailPassword, f.accessKey, f.secretKey, f.jwtSecret))
}

func gitFiles(w http.ResponseWriter, r *http.Request, _ fakes) {
	switch strings.ToLower(strings.TrimPrefix(r.URL.Path, "/.git/")) {
	case "head":
		text(w, http.StatusOK, "text/plain", "ref: refs/heads/main\n")
	case "config":
		text(w, http.StatusOK, "text/plain", "[core]\n\trepositoryformatversion = 0\n\tfilemode = true\n\tbare = false\n[remote \"origin\"]\n\turl = git@git.admin-portal.internal:platform/admin-portal.git\n\tfetch = +refs/heads/*:refs/remotes/origin/*\n[branch \"main\"]\n\tremote = origin\n\tmerge = refs/heads/main\n")
	default:
		notFoundPage(w, r, fakes{})
	}
}

func awsCredentials(w http.ResponseWriter, _ *http.Request, f fakes) {
	text(w, http.StatusOK, "text/plain", fmt.Sprintf("[default]\naws_access_key_id = %s\naws_secret_access_key = %s\nregion = eu-west-3\n", f.accessKey, f.secretKey))
}

func xmlrpc(w http.ResponseWriter, r *http.Request, _ fakes) {
	if r.Method != http.MethodPost {
		text(w, http.StatusMethodNotAllowed, "text/plain", "XML-RPC server accepts POST requests only.")
		return
	}
	text(w, http.StatusOK, "text/xml; charset=UTF-8", `<?xml version="1.0" encoding="UTF-8"?>
<methodResponse><fault><value><struct><member><name>faultCode</name><value><int>403</int></value></member><member><name>faultString</name><value><string>Incorrect username or password.</string></value></member></struct></value></fault></methodResponse>
`)
}

func wpUsers(w http.ResponseWriter, _ *http.Request, _ fakes) {
	text(w, http.StatusOK, "application/json; charset=UTF-8", `[{"id":1,"name":"admin","url":"","description":"","link":"/author/admin/","slug":"admin"}]`)
}

func actuator(w http.ResponseWriter, r *http.Request, f fakes) {
	switch strings.TrimSuffix(strings.ToLower(r.URL.Path), "/") {
	case "/actuator":
		text(w, http.StatusOK, "application/vnd.spring-boot.actuator.v3+json", `{"_links":{"self":{"href":"/actuator"},"health":{"href":"/actuator/health"},"env":{"href":"/actuator/env"},"info":{"href":"/actuator/info"}}}`)
	case "/actuator/health":
		text(w, http.StatusOK, "application/vnd.spring-boot.actuator.v3+json", `{"status":"UP"}`)
	case "/actuator/info":
		text(w, http.StatusOK, "application/vnd.spring-boot.actuator.v3+json", `{"app":{"name":"admin-portal","version":"2.4.1"}}`)
	case "/actuator/env":
		text(w, http.StatusOK, "application/vnd.spring-boot.actuator.v3+json", fmt.Sprintf(`{"activeProfiles":["prod"],"propertySources":[{"name":"applicationConfig","properties":{"spring.datasource.url":{"value":"jdbc:mysql://10.20.0.15:3306/portal"},"spring.datasource.username":{"value":"portal"},"spring.datasource.password":{"value":"%s"}}}]}`, f.dbPassword))
	default:
		text(w, http.StatusNotFound, "application/json", `{"status":404,"error":"Not Found"}`)
	}
}

func configJSON(w http.ResponseWriter, _ *http.Request, f fakes) {
	text(w, http.StatusOK, "application/json", fmt.Sprintf(`{"env":"production","api":{"baseUrl":"https://admin-portal.internal/api","token":"%s"},"db":{"host":"10.20.0.15","user":"portal","password":"%s"}}`, f.apiToken, f.dbPassword))
}

func apiUnauthorized(w http.ResponseWriter, _ *http.Request, _ fakes) {
	text(w, http.StatusUnauthorized, "application/json", `{"error":"unauthorized","message":"missing or invalid token"}`)
}

func notFoundPage(w http.ResponseWriter, r *http.Request, _ fakes) {
	html(w, http.StatusNotFound, fmt.Sprintf(`<!DOCTYPE HTML PUBLIC "-//IETF//DTD HTML 2.0//EN">
<html><head><title>404 Not Found</title></head><body><h1>Not Found</h1><p>The requested URL %s was not found on this server.</p><hr><address>Apache/2.4.58 (Ubuntu) Server at %s Port 443</address></body></html>
`, url.PathEscape(r.URL.Path), r.Host))
}
