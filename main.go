package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	"github.com/spf13/viper"
	"github.com/spy16/droplets/interfaces/mongo"
	"github.com/spy16/droplets/interfaces/rest"
	"github.com/spy16/droplets/interfaces/web"
	"github.com/spy16/droplets/pkg/graceful"
	"github.com/spy16/droplets/pkg/logger"
	"github.com/spy16/droplets/pkg/middlewares"
	"github.com/spy16/droplets/usecases/posts"
	"github.com/spy16/droplets/usecases/users"
)

func main() {
	viper.AutomaticEnv()
	viper.SetDefault("MONGO_URI", "mongodb://localhost/droplets")
	viper.SetDefault("LOG_LEVEL", "debug")
	viper.SetDefault("LOG_FORMAT", "text")
	viper.SetDefault("ADDR", ":8080")
	viper.SetDefault("STATIC_DIR", "./web/static/")
	viper.SetDefault("TEMPLATE_DIR", "./web/templates/")

	lg := logger.New(os.Stderr, viper.GetString("LOG_LEVEL"), viper.GetString("LOG_FORMAT"))

	db, closeSession, err := mongo.Connect(viper.GetString("MONGO_URI"), true)
	if err != nil {
		lg.Fatalf("failed to connect to mongodb: %v", err)
	}
	defer closeSession()

	lg.Debugf("setting up rest api service")
	userStore := mongo.NewUserStore(db)
	postStore := mongo.NewPostStore(db)

	userRegistration := users.NewRegistrar(lg, userStore)
	userRetriever := users.NewRetriever(lg, userStore)

	postPub := posts.NewPublication(lg, postStore, userStore)
	postRet := posts.NewRetriever(lg, postStore)

	restHandler := rest.New(lg, userRegistration, userRetriever, postRet, postPub)
	restHandler = middlewares.WithBasicAuth(middlewares.UserVerifierFunc(adminVerifier), lg, restHandler)

	webHandler, err := web.New(lg, web.Config{
		TemplateDir: viper.GetString("TEMPLATE_DIR"),
		StaticDir:   viper.GetString("STATIC_DIR"),
	})
	if err != nil {
		lg.Fatalf("failed to setup web handler: %v", err)
	}

	router := mux.NewRouter()
	router.PathPrefix("/api").Handler(http.StripPrefix("/api", restHandler))
	router.PathPrefix("/").Handler(webHandler)

	srv := server(lg, router)
	srv.Addr = viper.GetString("addr")
	lg.Infof("listening for requests on :8080...")
	if err := srv.ListenAndServe(); err != nil {
		lg.Fatalf("http server exited: %s", err)
	}
}

func server(lg logger.Logger, handler http.Handler) *graceful.Server {
	viper.SetDefault("GRACEFUL_TIMEOUT", 20*time.Second)
	timeout := viper.GetDuration("GRACEFUL_TIMEOUT")

	handler = withMiddlewares(handler, lg)
	srv := graceful.NewServer(handler, timeout, os.Interrupt)
	srv.Log = lg.Errorf

	return srv
}

func withMiddlewares(handler http.Handler, logger logger.Logger) http.Handler {
	handler = middlewares.WithRequestLogging(logger, handler)
	handler = middlewares.WithRecovery(logger, handler)
	return handler
}

func adminVerifier(ctx context.Context, name, secret string) bool {
	return secret == "secret@123"
}
