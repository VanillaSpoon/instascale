package controllers

import (
	"context"
	"fmt"
	"os"
	"strings"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	arbv1 "github.com/project-codeflare/multi-cluster-app-dispatcher/pkg/apis/controller/v1beta1"

	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *AppWrapperReconciler) scaleNodePool(ctx context.Context, aw *arbv1.AppWrapper, demandPerInstanceType map[string]int) (ctrl.Result, error) {
	connection, err := r.createOCMConnection()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating OCM connection: %v", err)
		return ctrl.Result{}, err
	}
	defer connection.Close()
	for userRequestedInstanceType := range demandPerInstanceType {
		replicas := demandPerInstanceType[userRequestedInstanceType]

		clusterNodePools := connection.ClustersMgmt().V1().Clusters().Cluster(r.ocmClusterID).NodePools()

		response, err := clusterNodePools.List().SendContext(ctx)
		if err != nil {
			return ctrl.Result{}, err
		}

		numberOfMachines := 0
		response.Items().Each(func(nodePool *cmv1.NodePool) bool {

			if nodePool.AWSNodePool().InstanceType() == userRequestedInstanceType && hasAwLabel(nodePool.Labels(), aw) {
				//if nodePool.AWSNodePool().InstanceType() == userRequestedInstanceType && hasAwLabelNode(nodePool, aw) {
				numberOfMachines = nodePool.Replicas()
				return false
			}
			return true
		})

		if numberOfMachines != replicas {
			m := make(map[string]string)
			m[aw.Name] = aw.Name
			klog.Infof("The instanceRequired array: %v", userRequestedInstanceType)

			nodePoolID := strings.ReplaceAll(aw.Name+"-"+userRequestedInstanceType, ".", "-")

			createNodePool, err := cmv1.NewNodePool().AWSNodePool(cmv1.NewAWSNodePool().InstanceType(userRequestedInstanceType)).ID(nodePoolID).Replicas(replicas).Labels(m).Build()
			if err != nil {
				klog.Errorf(`Error building NodePool: %v`, err)
			}
			klog.Infof("Built NodePool with instance type %v and name %v", userRequestedInstanceType, createNodePool.ID())
			response, err := clusterNodePools.Add().Body(createNodePool).SendContext(ctx)
			if err != nil {
				klog.Errorf(`Error creating NodePool: %v`, err)
			}
			klog.Infof("Created NodePool: %v", response)
		}
	}
	return ctrl.Result{Requeue: false}, nil
}

func (r *AppWrapperReconciler) deleteNodePool(ctx context.Context, aw *arbv1.AppWrapper) (ctrl.Result, error) {
	connection, err := r.createOCMConnection()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating OCM connection: %v", err)
		return ctrl.Result{}, err
	}
	defer connection.Close()

	nodePoolsConnection := connection.ClustersMgmt().V1().Clusters().Cluster(r.ocmClusterID).NodePools().List()

	nodePoolsListResponse, _ := nodePoolsConnection.Send()
	nodePoolsList := nodePoolsListResponse.Items()
	nodePoolsList.Range(func(index int, item *cmv1.NodePool) bool {
		id, _ := item.GetID()
		if strings.Contains(id, aw.Name) {
			targetNodePool, err := connection.ClustersMgmt().V1().Clusters().Cluster(r.ocmClusterID).NodePools().NodePool(id).Delete().SendContext(ctx)
			if err != nil {
				klog.Infof("Error deleting target nodepool %v", targetNodePool)
			}
			klog.Infof("Successfully Scaled down target nodepool %v", id)
		}
		return true
	})
	return ctrl.Result{Requeue: false}, nil
}

func (r *AppWrapperReconciler) nodePoolExists() (bool, error) {
	connection, err := r.createOCMConnection()
	if err != nil {
		return false, fmt.Errorf("error creating OCM connection: %w", err)
	}
	defer connection.Close()

	nodePools := connection.ClustersMgmt().V1().Clusters().Cluster(r.ocmClusterID).NodePools()
	return nodePools != nil, nil
}
