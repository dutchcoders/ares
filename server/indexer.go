package server

import (
	"context"
	"time"
	"net/url"
	"github.com/pborman/uuid"
	"gopkg.in/olivere/elastic.v5"
)

func (p *Server) indexer() {
	log.Info("Indexer started...")
	defer log.Info("Indexer stopped...")

	u, err := url.Parse(p.ElasticsearchURL)
	if err != nil {
	   log.Error("Error parsing url: %s", p.ElasticsearchURL)
		panic(err)
	}

	if u.Path == "" || u.Path[1:] == "" {
            log.Error("Index is not set in elasticsearch_url: ", p.ElasticsearchURL)
        }
   
	es, err := elastic.NewClient(elastic.SetURL(u.Host), elastic.SetSniff(false))
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
				Type("pairs").
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
