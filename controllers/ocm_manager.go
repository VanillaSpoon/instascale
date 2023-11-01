package controllers

import (
	"context"
	"fmt"
	ocmsdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	configv1 "github.com/openshift/api/config/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	"os"
)

func (r *AppWrapperReconciler) createOCMConnection() (*ocmsdk.Connection, error) {
	logger, err := ocmsdk.NewGoLoggerBuilder().
		Debug(true).
		Build()
	if err != nil {
		return nil, fmt.Errorf("can't build logger: %v", err)
	}

	connection, err := ocmsdk.NewConnectionBuilder().
		Logger(logger).
		Tokens(r.ocmToken).
		Build()
	if err != nil {
		return nil, fmt.Errorf("can't build connection: %v", err)
	}

	r.ocmConnection = connection
	return r.ocmConnection, nil
}

func (r *AppWrapperReconciler) getOCMClusterID() error {
	cv := &configv1.ClusterVersion{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{Name: "version"}, cv)
	if err != nil {
		return fmt.Errorf("can't get clusterversion: %v", err)
	}

	internalClusterID := string(cv.Spec.ClusterID)

	ctx := context.Background()

	connection, err := r.createOCMConnection()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating OCM connection: %v", err)
	}
	defer connection.Close()

	// Get the client for the resource that manages the collection of clusters:
	collection := r.ocmConnection.ClustersMgmt().V1().Clusters()

	response, err := collection.List().Search(fmt.Sprintf("external_id = '%s'", internalClusterID)).Size(1).Page(1).SendContext(ctx)
	if err != nil {
		klog.Errorf(`Error getting cluster id: %v`, err)
	}

	response.Items().Each(func(cluster *cmv1.Cluster) bool {
		r.ocmClusterID = cluster.ID()
		fmt.Printf("%s - %s - %s\n", cluster.ID(), cluster.Name(), cluster.State())
		return true
	})
	return nil
}

// closes the OCM connection
func (r *AppWrapperReconciler) Close() {
	if r.ocmConnection != nil {
		r.ocmConnection.Close()
	}
}
