package database

import (
	"net/url"

	mgo "gopkg.in/mgo.v2"
)

type Database struct {
	*mgo.Database

	Campaigns *mgo.Collection
	Events    *mgo.Collection
	Emails    *mgo.Collection
	Users     *mgo.Collection
}

func Open(s string) (*Database, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, err
	}

	session, err := mgo.Dial(u.Host)
	if err != nil {
		return nil, err
	}

	session.SetMode(mgo.Monotonic, true)

	d := Database{}

	d.Database = session.DB(u.Path[1:])

	d.Users = d.Database.C("users")
	d.Campaigns = d.Database.C("campaigns")
	d.Events = d.Database.C("events")
	d.Emails = d.Database.C("emails")
	return &d, nil
}
