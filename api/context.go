package api

import (
	"encoding/json"
	"net/http"

	"github.com/dutchcoders/ares/api/errors"
	_ "github.com/labstack/gommon/log"
)

func init() {
}

type AfterFunc func()

type Context struct {
	w http.ResponseWriter
	r *http.Request

	bodyWritten bool
}

type ContextFunc func(*Context) error

func (api *API) ContextHandlerFunc(h ContextFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := Context{
			r: r,
			w: w,
		}

		var err error
		defer func() {
			if err == nil {
				if ctx.bodyWritten {
					w.WriteHeader(http.StatusOK)
				} else {
					w.WriteHeader(http.StatusNoContent)
				}
				return
			}

			log.Error(err.Error())

			switch err.(type) {
			case errors.APIError:
				w.WriteHeader(err.(errors.APIError).Code())
				json.NewEncoder(w).Encode(err)
			default:
				http.Error(w, err.Error(), 500)
			}
		}()

		err = h(&ctx)
		return
	}
}
