package models

import (
	"time"

	"gopkg.in/mgo.v2/bson"
)

type Campaign struct {
	CampaignID bson.ObjectId `json:"campaign_id" bson:"_id,omitempty"`

	Title string `json:"title" bson:"title"`

	Users []bson.ObjectId `json:"users" bson:"users"`
}

type User struct {
	UserID bson.ObjectId `json:"user_id" bson:"_id,omitempty"`

	Firstname string `json:"first_name" bson:"first_name"`
	Lastname  string `json:"last_name" bson:"last_name"`
	Email     string `json:"email" bson:"email"`

	EmailsSent []struct {
		EmailID bson.ObjectId `json:"email_id" bson:"email_id"`
		Token   bson.ObjectId `json:"token" bson:"token"`
		Date    time.Time     `json:"date" bson:"date"`
	} `json:"emails_sent" bson:"emails_sent"`
}

type Email struct {
	EmailID bson.ObjectId `json:"email_id" bson:"_id,omitempty"`

	CampaignID bson.ObjectId `json:"campaign_id" bson:"campaign_id"`

	Subject string `json:"subject" bson:"subject"`
}

type Event struct {
	EventID bson.ObjectId `bson:"_id,omitempty"`

	EmailID    bson.ObjectId `bson:"email_id"`
	CampaignID bson.ObjectId `bson:"campaign_id"`
	UserID     bson.ObjectId `bson:"user_id"`

	Date        time.Time `bson:"date"`
	Category    string    `bson:"category"`
	Description string    `bson:"description"`

	Method    string `json:"method" bson:"method"`
	URL       string `json:"url" bson:"url"`
	UserAgent string `json:"user_agent" bson:"user_agent"`
	Referer   string `json:"referer" bson:"referer"`

	// Values map[string][]string `json:"values" bson:"values"`
	Data interface{} `json:"data bson:"data"`
}
