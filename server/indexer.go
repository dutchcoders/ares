package server

import (
	"context"
	"net/url"
	"time"

	_ "github.com/labstack/gommon/log"
	"github.com/pborman/uuid"
	"gopkg.in/olivere/elastic.v5"
)

type Indexabler interface {
	ID() string
	Type() string
}

func (p *Server) indexer() {
	log.Info("Indexer started...")
	defer log.Info("Indexer stopped...")

	u, err := url.Parse(p.ElasticsearchURL)
	if err != nil {
		panic(err)
	}

	es, err := elastic.NewClient(elastic.SetURL(p.ElasticsearchURL), elastic.SetSniff(false))
	if err != nil {
		panic(err)
	}

	bulk := es.Bulk()

	count := 0
	for {
		select {
		case doc := <-p.index:
			docId := uuid.NewUUID()

			bulk = bulk.Add(elastic.NewBulkIndexRequest().
				Index(u.Path[1:]).
				Type("event").
				Id(docId.String()).
				Doc(doc),
			)

			log.Debugf("Indexed message with id %s", docId.String())

			// pretty.Print(doc)
			if bulk.NumberOfActions() < 10 {
				continue
			}
		case <-time.After(time.Second * 10):
		}

		if bulk.NumberOfActions() == 0 {
		} else if response, err := bulk.Do(context.Background()); err != nil {
			log.Errorf("Error indexing: %s", err.Error())
		} else {
			indexed := response.Indexed()
			count += len(indexed)

			log.Infof("Bulk indexing: %d total %d.\n", len(indexed), count)
		}
	}
}
