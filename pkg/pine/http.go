package pine

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"

	api "github.com/mpoegel/pine/pkg/api"
	tree "github.com/mpoegel/pine/pkg/tree"
)

type TreeKeeper interface {
	StartTree(ctx context.Context, name string) error
	StopTree(ctx context.Context, name string) error
	RestartTree(ctx context.Context, name string) error
	GetTreeStatus(ctx context.Context, name string) (*tree.Status, error)
	ListTrees(ctx context.Context) ([]*tree.Status, error)
	RotateTreeLog(ctx context.Context, name string) error
}

type HttpServer struct {
	server http.Server
	keeper TreeKeeper
}

func NewHttpServer(keeper TreeKeeper) *HttpServer {
	httpServer := &HttpServer{
		server: http.Server{},
		keeper: keeper,
	}

	return httpServer
}

func (s *HttpServer) Start(ctx context.Context, ln net.Listener) error {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /tree/start/{treeName}", s.startTree(ctx))
	mux.HandleFunc("POST /tree/stop/{treeName}", s.stopTree(ctx))
	mux.HandleFunc("POST /tree/restart/{treeName}", s.restartTree(ctx))
	mux.HandleFunc("POST /tree/logrotate/{treeName}", s.rotateTreeLog(ctx))
	mux.HandleFunc("GET /tree/{treeName}", s.treeStatus(ctx))
	mux.HandleFunc("GET /tree", s.listTrees(ctx))
	s.server.Handler = mux

	errChan := make(chan error, 1)
	go func() {
		errChan <- s.server.Serve(ln)
	}()

	select {
	case <-ctx.Done():
		s.server.Shutdown(context.Background())
		return nil
	case err := <-errChan:
		return err
	}
}

func (s *HttpServer) startTree(ctx context.Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("treeName")
		if s.keeper.StartTree(ctx, name) != nil {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}
}

func (s *HttpServer) stopTree(ctx context.Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("treeName")
		if s.keeper.StopTree(ctx, name) != nil {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}
}

func (s *HttpServer) restartTree(ctx context.Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("treeName")
		if s.keeper.RestartTree(ctx, name) != nil {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}
}

func (s *HttpServer) treeStatus(ctx context.Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("treeName")
		status, err := s.keeper.GetTreeStatus(ctx, name)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		resp := api.TreeStatusResponse{
			TreeName:   name,
			State:      string(status.State),
			LastChange: uint64(status.LastChange.Unix()),
			Uptime:     uint64(status.Uptime.Seconds()),
		}
		w.Header().Add("content-type", "application/json")
		encoder := json.NewEncoder(w)
		if err := encoder.Encode(resp); err != nil {
			slog.Error("could not encode tree status api response", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
}

func (s *HttpServer) listTrees(ctx context.Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		statusList, err := s.keeper.ListTrees(ctx)
		if err != nil {
			w.WriteHeader(http.StatusPartialContent)
		}
		resp := &api.ListTreesResponse{
			Trees: []api.TreeStatusResponse{},
		}
		for _, status := range statusList {
			resp.Trees = append(resp.Trees, api.TreeStatusResponse{
				TreeName:   status.For.Name,
				State:      string(status.State),
				LastChange: uint64(status.LastChange.Unix()),
				Uptime:     uint64(status.Uptime.Seconds()),
			})
		}
		w.Header().Add("content-type", "application/json")
		encoder := json.NewEncoder(w)
		if err := encoder.Encode(resp); err != nil {
			slog.Error("could not encode tree status api response", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
}

func (s *HttpServer) rotateTreeLog(ctx context.Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("treeName")
		if s.keeper.RotateTreeLog(ctx, name) != nil {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}
}
