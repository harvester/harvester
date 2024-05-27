package services

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/services/containerservice/mgmt/2020-11-01/containerservice"
	"github.com/Azure/go-autorest/autorest"
)

type AgentPoolsClientInterface interface {
	CreateOrUpdate(ctx context.Context, resourceGroupName string, clusterName string, agentPoolName string, parameters containerservice.AgentPool) (containerservice.AgentPoolsCreateOrUpdateFuture, error)
	Delete(ctx context.Context, resourceGroupName string, clusterName string, agentPoolName string) (containerservice.AgentPoolsDeleteFuture, error)
}

type agentPoolClient struct {
	agentPoolClient containerservice.AgentPoolsClient
}

func NewAgentPoolClient(authorizer autorest.Authorizer, baseURL, subscriptionID string) (*agentPoolClient, error) {
	client := containerservice.NewAgentPoolsClientWithBaseURI(baseURL, subscriptionID)
	client.Authorizer = authorizer
	return &agentPoolClient{
		agentPoolClient: client,
	}, nil
}

func (cl *agentPoolClient) CreateOrUpdate(ctx context.Context, resourceGroupName string, clusterName string, agentPoolName string, parameters containerservice.AgentPool) (containerservice.AgentPoolsCreateOrUpdateFuture, error) {
	return cl.agentPoolClient.CreateOrUpdate(ctx, resourceGroupName, clusterName, agentPoolName, parameters)
}

func (cl *agentPoolClient) Delete(ctx context.Context, resourceGroupName string, clusterName string, agentPoolName string) (containerservice.AgentPoolsDeleteFuture, error) {
	return cl.agentPoolClient.Delete(ctx, resourceGroupName, clusterName, agentPoolName)
}
