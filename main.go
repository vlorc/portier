package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/tls"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"html/template"
	"io"
	"io/fs"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"
)

//go:embed views
var views embed.FS

func mailer(addr *string, secure *bool, user, pass *string) func(string, *template.Template, map[string]interface{}) error {
	return func(to string, tmpl *template.Template, data map[string]interface{}) error {
		var body bytes.Buffer
		data["to"] = to
		data["from"] = *user
		if err := tmpl.ExecuteTemplate(&body, "captcha.html", data); nil != err {
			return err
		}
		host, port, err := net.SplitHostPort(*addr)
		var conn net.Conn
		if port == "465" || *secure {
			conn, err = tls.Dial("tcp", *addr, &tls.Config{InsecureSkipVerify: *secure})
		} else {
			conn, err = net.Dial("tcp", *addr)
		}
		if err != nil {
			return err
		}
		cli, err := smtp.NewClient(conn, host)
		if nil != err {
			conn.Close()
			return err
		}
		defer cli.Close()
		if ok, _ := cli.Extension("AUTH"); ok {
			if err = cli.Auth(smtp.PlainAuth("", *user, *pass, host)); nil != err {
				return err
			}
		}
		if err = cli.Mail(*user); nil != err {
			return err
		}
		if err = cli.Rcpt(to); nil != err {
			return err
		}
		w, err := cli.Data()
		if nil != err {
			return err
		}
		if _, err = io.Copy(w, &body); nil != err {
			return err
		}
		if err = w.Close(); nil != err {
			return err
		}
		return cli.Quit()
	}
}

func nonce() string {
	v, _ := rand.Int(rand.Reader, big.NewInt(8999))
	return strconv.FormatInt(v.Int64()+1000, 10)
}

func tostring(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

func tobytes(s string) []byte {
	return *(*[]byte)(unsafe.Pointer(&s))
}

func verify(secret string, value string) bool {
	i := strings.Index(value, ".")
	if i <= 0 {
		return false
	}
	if t, _ := strconv.ParseInt(value[:i], 16, 63); t < time.Now().Unix() {
		return false
	}
	var digest [32]byte
	h := hmac.New(md5.New, tobytes(secret))
	h.Write(tobytes(value[:i]))
	hex.Encode(digest[16:], h.Sum(digest[:0])[4:12])
	return value[i+1:] == tostring(digest[16:32])
}

func signature(secret string, ttl int) string {
	var digest [16]byte
	var cache [64]byte
	b := strconv.AppendInt(cache[:0], time.Now().Unix()+int64(ttl), 16)
	h := hmac.New(md5.New, tobytes(secret))
	h.Write(b)
	b = append(b, '.')
	n := hex.Encode(cache[len(b):], h.Sum(digest[:0])[4:12])
	return string(cache[:len(b)+n])
}

func session(secret, domain, name *string, expires *int) (func(*http.Request) bool, func(http.ResponseWriter) bool) {
	return func(req *http.Request) bool {
			if c, _ := req.Cookie(*name); nil != c {
				return verify(*secret, c.Value)
			}
			return false
		},
		func(resp http.ResponseWriter) bool {
			http.SetCookie(resp, &http.Cookie{
				Name:     *name,
				Value:    signature(*secret, *expires),
				Domain:   *domain,
				MaxAge:   *expires,
				HttpOnly: true,
			})
			return true
		}
}

func hostname(redirect string) string {
	if u, _ := url.Parse(redirect); nil != u {
		return u.Hostname()
	}
	return ""
}

func match(str *string) func(string) bool {
	re := regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")
	return func(val string) bool {
		return re.MatchString(val) && strings.HasSuffix(val, *str)
	}
}

func querier(key *string) func(*http.Request) string {
	return func(req *http.Request) string {
		if "" != req.URL.RawQuery {
			return req.URL.Query().Get(*key)
		}
		return ""
	}
}

type filesystem []fs.FS
type subsystem struct {
	fsys fs.FS
	dir  string
}

func (f *subsystem) Open(name string) (fs.File, error) {
	return f.fsys.Open(path.Join(f.dir, name))
}

func (f filesystem) Open(name string) (file fs.File, err error) {
	for i := range f {
		if file, err = f[i].Open(name); nil == err {
			break
		}
	}
	return
}

func (f filesystem) Json(name string, val interface{}) error {
	if buf, err := fs.ReadFile(f, name); nil != err {
		return err
	} else {
		return json.Unmarshal(buf, val)
	}
}

func main() {
	lang := flag.String("lang", "en", "language")
	fsys := filesystem{os.DirFS("."), &subsystem{views, "views"}}
	addr := flag.String("addr", "127.0.0.1:4567", "listen address")
	uri := flag.String("path", "/login", "login path")
	check, setup := session(
		flag.String("cookie.secret", "portier", "cookie secret"),
		flag.String("cookie.domain", "", "cookie domain"),
		flag.String("cookie.name", "portier", "cookie name"),
		flag.Int("cookie.expires", 3600, "cookie expires"),
	)
	query := querier(flag.String("redirect", "redirect", "redirect key"))
	send := mailer(
		flag.String("mail.addr", "", "mail address"),
		flag.Bool("mail.ssl", false, "mail ssl"),
		flag.String("mail.user", "", "mail username"),
		flag.String("mail.pass", "", "mail password"),
	)
	valid := match(flag.String("mail.suffix", "", "mail suffix"))
	flag.Parse()

	mapping := sync.Map{}
	dict := map[string]string{}

	tmpl, err := template.New("").
		Funcs(template.FuncMap{
			"base64": func(v string) template.HTML {
				return template.HTML(base64.StdEncoding.EncodeToString(tobytes(v)))
			},
		}).
		ParseFS(fsys, "login.html", "captcha.html")
	if nil != err {
		log.Println(err.Error())
	}
	if nil != fsys.Json("lang.json", &dict) {
		fsys.Json(*lang+".json", &dict)
	}

	login := func(resp http.ResponseWriter, req *http.Request, redirect string) bool {
		mail := req.FormValue("mail")
		code := req.FormValue("code")
		data := map[string]interface{}{"dict": dict, "mail": mail, "domain": hostname(redirect), "now": time.Now().Format("2006-01-02 15:04:05")}
		switch {
		case http.MethodGet == req.Method:
		case !valid(mail):
			data["message"] = dict["mail.reject"]
		case "" == code:
			code = nonce()
			mapping.Store(mail, code)
			data["code"] = code
			data["required"] = "required"
			data["message"] = dict["captcha.sent"]
			log.Println("send mail:", mail, "code:", code, "error:", send(mail, tmpl, data))
		default:
			if v, ok := mapping.LoadAndDelete(mail); ok && code == v.(string) {
				return setup(resp)
			}
			data["message"] = dict["captcha.failed"]
		}
		tmpl.ExecuteTemplate(resp, "login.html", data)
		return false
	}

	http.ListenAndServe(*addr, http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		if redirect, page := query(req), *uri == req.URL.Path; check(req) || (page && login(resp, req, redirect)) {
			if "" != redirect {
				http.Redirect(resp, req, redirect, http.StatusFound)
			}
		} else if !page {
			resp.WriteHeader(http.StatusUnauthorized)
		}
	}))
}
