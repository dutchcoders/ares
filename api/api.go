package api

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	"mime"
	"net"
	"net/http"
	"net/smtp"
	"path"
	"time"

	"gopkg.in/mgo.v2/bson"

	handlers "github.com/dutchcoders/ares/api/handlers"
	"github.com/dutchcoders/ares/database"
	"github.com/gorilla/mux"
	"github.com/mohamedattahri/mail"

	// "github.com/olivere/elastic"
	"github.com/op/go-logging"

	model "github.com/dutchcoders/ares/model"
	// "github.com/mattbaird/elastigo/lib"
)

var log = logging.MustGetLogger("api")

type API struct {
	db *database.Database
}

func New(db *database.Database) *API {
	return &API{
		db: db,
	}
}

func (api *API) campaignsPostHandler(ctx *Context) error {
	req := struct {
		Title string `json:"title"`
	}{}

	if err := json.NewDecoder(ctx.r.Body).Decode(&req); err != nil {
		return err
	}

	c := model.Campaign{
		Users: []bson.ObjectId{},
	}

	if err := Merge(&c, req); err != nil {
		return err
	}

	c.CampaignID = bson.NewObjectId()

	if _, err := api.db.Campaigns.UpsertId(c.CampaignID, &c); err != nil {
		log.Errorf("Error during upserting: %s", err.Error())
	}

	return json.NewEncoder(ctx.w).Encode(c)
}

func (api *API) usersPostHandler(ctx *Context) error {
	req := struct {
		Email string `json:"email"`
	}{}

	if err := json.NewDecoder(ctx.r.Body).Decode(&req); err != nil {
		return err
	}

	u := model.User{}

	if err := Merge(&u, req); err != nil {
		return err
	}

	u.UserID = bson.NewObjectId()

	if _, err := api.db.Users.UpsertId(u.UserID, &u); err != nil {
		log.Errorf("Error during upserting: %s", err.Error())
	}

	return json.NewEncoder(ctx.w).Encode(u)
}

func (api *API) emailSendHandler(ctx *Context) error {
	req := struct {
		EmailID bson.ObjectId `json:"email_id"`
		Email   string        `json:"email"`
		// UserID bson.ObjectID `json:"user_id"`
	}{}

	if err := json.NewDecoder(ctx.r.Body).Decode(&req); err != nil {
		return err
	}

	/*
		e := model.Email{}

		if err := Merge(&e, req); err != nil {
			return err
		}

		e.EmailID = bson.NewObjectId()

		if _, err := api.db.Emails.UpsertId(e.EmailID, &e); err != nil {
			log.Errorf("Error during upserting: %s", err.Error())
		}

	*/
	email := model.Email{}
	if err := api.db.Emails.FindId(req.EmailID).One(&email); err != nil {
		log.Errorf("Could not find email: %s", err.Error())
		return err
	}

	campaign := model.Campaign{}
	if err := api.db.Campaigns.FindId(email.CampaignID).One(&campaign); err != nil {
		log.Errorf("Could not find campaign: %s", err.Error())
	}

	user := model.User{}
	if err := api.db.Users.Find(bson.M{"email": req.Email}).One(&user); err != nil {
		log.Errorf("Could not find user: %s", err.Error())
		return err
	}

	for _, emailSent := range user.EmailsSent {
		fmt.Println(emailSent.EmailID, email.EmailID)
		if emailSent.EmailID == email.EmailID {
			return errors.New("Email already sent.")
		}
	}

	token := bson.NewObjectId()

	// send email

	// channel!

	m := mail.NewMessage()
	m.SetFrom(&mail.Address{"NS parkeren", "info@ns-parkeren.nl"})

	m.To().Add(&mail.Address{"", user.Email})

	m.SetSubject(fmt.Sprintf("Onderzoek parkeerproblemen rondom stations Den Haag"))

	mixed := mail.NewMultipart("multipart/mixed", m)

	alternative, _ := mixed.AddMultipart("multipart/alternative")

	data := map[string]interface{}{
		"Token": token.Hex(),
		"User":  user,
	}

	for _, p := range []string{"template.txt", "template.html"} {
		contentType := mime.TypeByExtension(path.Ext(p))

		if contentType == "" {
			contentType = "text/plain"
		}

		if templ, err := ioutil.ReadFile(p); err != nil {
			panic(err)
		} else {
			var t = template.Must(template.New("name").Parse(string(templ)))

			var body bytes.Buffer

			if err := t.Execute(&body, data); err != nil {
				panic(err)
			}

			alternative.AddText(contentType, &body)
		}
	}

	alternative.Close()

	mixed.Close()

	// Connect to the SMTP Server
	servername := "mail.business-facilitate.com:465"

	host, _, _ := net.SplitHostPort(servername)

	// TLS config
	tlsconfig := &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         host,
	}

	// Here is the key, you need to call tls.Dial instead of smtp.Dial
	// for smtp servers running on 465 that require an ssl connection
	// from the very beginning (no starttls)
	conn, err := tls.Dial("tcp", servername, tlsconfig)
	if err != nil {
		log.Panic(err)
	}

	c, err := smtp.NewClient(conn, host)
	if err != nil {
		log.Panic(err)
	}

	c.Auth(smtp.PlainAuth(
		"",
		"info@ns-parkeren.nl",
		"Ofni2017!",
		"mail.business-facilitate.com",
	))

	// To && From
	if err = c.Mail("info@ns-parkeren.nl"); err != nil {
		log.Panic(err)
	}

	if err = c.Rcpt(user.Email); err != nil {
		log.Panic(err)
	}

	wc, err := c.Data()
	if err != nil {
		log.Fatal(err)
	}
	defer wc.Close()

	if _, err = wc.Write(m.Bytes()); err != nil {
		log.Fatal(err)
	}

	if err := c.Quit(); err != nil {
		fmt.Println(err.Error())
	}

	if err := api.db.Users.UpdateId(user.UserID, bson.M{"$addToSet": bson.M{"emails_sent": bson.M{
		"email_id": email.EmailID,
		"token":    token,
		"date":     time.Now(),
	}}}); err != nil {
		log.Errorf("Could not update campaign: %s", err.Error())
	}

	return nil
	//	json.NewEncoder(ctx.w).Encode(struct {})
}

func (api *API) emailsPostHandler(ctx *Context) error {
	req := struct {
		CampaignID bson.ObjectId `json:"campaign_id"`
		Subject    string        `json:"subject"`
	}{}

	if err := json.NewDecoder(ctx.r.Body).Decode(&req); err != nil {
		return err
	}

	e := model.Email{}

	if err := Merge(&e, req); err != nil {
		return err
	}

	e.EmailID = bson.NewObjectId()

	if _, err := api.db.Emails.UpsertId(e.EmailID, &e); err != nil {
		log.Errorf("Error during upserting: %s", err.Error())
	}

	return json.NewEncoder(ctx.w).Encode(e)
}

func (api *API) campaignUserPostHandler(ctx *Context) error {
	req := struct {
		CampaignID bson.ObjectId `json:"campaign_id"`
		UserID     bson.ObjectId `json:"user_id"`
		Email      string        `json:"email"`
	}{}

	if err := json.NewDecoder(ctx.r.Body).Decode(&req); err != nil {
		return err
	}

	campaign := model.Campaign{}
	if err := api.db.Campaigns.FindId(req.CampaignID).One(&campaign); err != nil {
		log.Errorf("Could not find campaign: %s", err.Error())
	}

	user := model.User{}

	q := bson.M{"_id": req.UserID}
	if req.Email != "" {
		q = bson.M{"email": req.Email}
	}

	if err := api.db.Users.Find(q).One(&user); err != nil {
		log.Errorf("Could not find user: %s", err.Error())
		return err
	}

	fmt.Printf("%+v\n", user)
	if err := api.db.Campaigns.UpdateId(campaign.CampaignID, bson.M{"$addToSet": bson.M{"users": user.UserID}}); err != nil {
		log.Errorf("Could not update campaign: %s", err.Error())
	}

	return nil // json.NewEncoder(ctx.w).Encode(e)
}

func (api *API) Serve() {
	r := mux.NewRouter()

	httpAddr := "127.0.0.1:5800"

	sr := r.PathPrefix("/v1").Subrouter()

	sr.HandleFunc("/users", api.ContextHandlerFunc(api.usersPostHandler)).Methods("POST")

	sr.HandleFunc("/campaigns", api.ContextHandlerFunc(api.campaignsPostHandler)).Methods("POST")
	sr.HandleFunc("/campaign/user", api.ContextHandlerFunc(api.campaignUserPostHandler)).Methods("POST")

	sr.HandleFunc("/emails", api.ContextHandlerFunc(api.emailsPostHandler)).Methods("POST")
	sr.HandleFunc("/email/send", api.ContextHandlerFunc(api.emailSendHandler)).Methods("POST")

	/*
		sr.HandleFunc("/messages", api.ContextHandlerFunc(api.messagesHandler)).Methods("GET")
		sr.HandleFunc("/channels", api.ContextHandlerFunc(api.channelsHandler)).Methods("GET")
		sr.HandleFunc("/users", api.ContextHandlerFunc(api.usersHandler)).Methods("GET")
		sr.HandleFunc("/team", api.ContextHandlerFunc(api.teamHandler)).Methods("GET")

		r.NotFoundHandler = http.HandlerFunc(notFoundHandler)
	*/
	_ = sr

	var handler http.Handler = r

	// install middlewares
	handler = handlers.LoggingHandler(handler)
	handler = handlers.RecoverHandler(handler)
	handler = handlers.RedirectHandler(handler)
	handler = handlers.CorsHandler(handler)

	if err := http.ListenAndServe(httpAddr, handler); err != nil {
		log.Fatalf("ListenAndServe %s: %v", httpAddr, err)
	}
}
