package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/awesomenix/azk/pkg/apis"
	enginev1alpha1 "github.com/awesomenix/azk/pkg/apis/engine/v1alpha1"
	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	corev1 "k8s.io/api/core/v1"
)

var WebCmd = &cobra.Command{
	Use:   "web",
	Short: "dashboard for target cluster",
	Long:  `dashboard for target cluster`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := RunWeb(); err != nil {
			log.Error(err, "failed to create web instance")
			os.Exit(1)
		}
	},
}

func init() {
	RootCmd.AddCommand(WebCmd)
}

func RunWeb() error {
	apis.AddToScheme(scheme.Scheme)
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	workDir, _ := os.Getwd()
	filesDir := filepath.Join(workDir, "dashboard")
	FileServer(r, "/dashboard", http.Dir(filesDir))

	r.Get("/", http.RedirectHandler("/dashboard", 301).ServeHTTP)

	r.Get("/events", Events)

	r.Mount("/clusters", clusterResource{}.Routes())

	http.ListenAndServe(":8080", r)
	return nil
}

// FileServer conveniently sets up a http.FileServer handler to serve
// static files from a http.FileSystem.
func FileServer(r chi.Router, path string, root http.FileSystem) {
	if strings.ContainsAny(path, "{}*") {
		panic("FileServer does not permit URL parameters.")
	}

	fs := http.StripPrefix(path, http.FileServer(root))

	if path != "/" && path[len(path)-1] != '/' {
		r.Get(path, http.RedirectHandler(path+"/", 301).ServeHTTP)
		path += "/"
	}
	path += "*"

	r.Get(path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fs.ServeHTTP(w, r)
	}))
}

type clusterResource struct{}

// Routes creates a REST router for the clusters resource
func (cr clusterResource) Routes() chi.Router {
	r := chi.NewRouter()
	// r.Use() // some middleware..

	r.Get("/", cr.List)
	// r.Post("/", cr.Create)
	// r.Put("/", cr.Delete)

	r.Route("/{id}", func(r chi.Router) {
		r.Get("/", cr.Get)
		// r.Put("/", cr.Update)
		// r.Delete("/", cr.Delete)
		// r.Get("/sync", cr.Sync)
	})

	return r
}

type clusterListType struct {
	Name              string `json:"name,omitempty"`
	Region            string `json:"region,omitempty"`
	ResourceGroup     string `json:"resourceGroup,omitempty"`
	SubscriptionID    string `json:"subscriptionID,omitempty"`
	KubernetesVersion string `json:"kubernetesVersion,omitempty"`
	Status            string `json:"status,omitempty"`
}

type eventListType struct {
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	LastSeen  string `json:"lastSeen,omitempty"`
	Type      string `json:"type,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Message   string `json:"message,omitempty"`
}

// List lists the clusters in bootstrap cluster
func (cr clusterResource) List(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.GetConfig()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get config %v", err), 400)
		return
	}

	crClient, err := client.New(cfg, client.Options{})
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get client %v", err), 400)
		return
	}

	clusterItemList := &enginev1alpha1.ClusterList{}
	err = crClient.List(context.Background(), &client.ListOptions{}, clusterItemList)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get cluster list %v", err), 400)
		return
	}
	scheme.Scheme.Default(clusterItemList)

	clusterList := []clusterListType{}

	for _, clusterItem := range clusterItemList.Items {
		status := clusterItem.Status.ProvisioningState

		clusterList = append(clusterList, clusterListType{
			Name:              clusterItem.Name,
			Region:            clusterItem.Spec.GroupLocation,
			ResourceGroup:     clusterItem.Spec.GroupName,
			SubscriptionID:    clusterItem.Spec.SubscriptionID,
			KubernetesVersion: clusterItem.Spec.BootstrapKubernetesVersion,
			Status:            status,
		})
	}

	clusterListJSON, err := json.Marshal(clusterList)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to fetch json %v", err), 400)
		return
	}
	w.Write(clusterListJSON)
}

// Get gets a specific cluster from bootstrap cluster
func (cr clusterResource) Get(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("cluster get"))
}

// Events gets all the events from bootstrap kubernetes cluster for now
func Events(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.GetConfig()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get config %v", err), 400)
		return
	}

	bClient, err := client.New(cfg, client.Options{})
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get client %v", err), 400)
		return
	}

	bEventList := corev1.EventList{}
	err = bClient.List(context.Background(), &client.ListOptions{}, &bEventList)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get event list %v", err), 400)
		return
	}

	eventList := []eventListType{}

	for _, eventItem := range bEventList.Items {
		lastSeen := time.Now().Sub(eventItem.LastTimestamp.Time)
		eventList = append(eventList, eventListType{
			Name:      eventItem.Name,
			Namespace: eventItem.Namespace,
			LastSeen:  lastSeen.String(),
			Type:      eventItem.Type,
			Reason:    eventItem.Reason,
			Kind:      eventItem.InvolvedObject.Kind,
			Message:   eventItem.Message,
		})
	}

	eventListJSON, err := json.Marshal(eventList)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to fetch json %v", err), 400)
		return
	}
	w.Write(eventListJSON)
}
