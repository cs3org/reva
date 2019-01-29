package main

import (
	"fmt"

	"github.com/pkg/errors"

	appproviderv0alphapb "github.com/cernbox/go-cs3apis/cs3/appprovider/v0alpha"
	appregistryv0alphapb "github.com/cernbox/go-cs3apis/cs3/appregistry/v0alpha"
	authv0alphapb "github.com/cernbox/go-cs3apis/cs3/auth/v0alpha"
	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
	storagebrokerv0alphapb "github.com/cernbox/go-cs3apis/cs3/storagebroker/v0alpha"
	storageproviderv0alphapb "github.com/cernbox/go-cs3apis/cs3/storageprovider/v0alpha"

	"google.golang.org/grpc"
)

func getAppProviderClient(host string) (appproviderv0alphapb.AppProviderServiceClient, error) {
	conn, err := getConnToHost(host)
	if err != nil {
		return nil, err
	}
	return appproviderv0alphapb.NewAppProviderServiceClient(conn), nil
}
func getStorageBrokerClient() (storagebrokerv0alphapb.StorageBrokerServiceClient, error) {
	conn, err := getConn()
	if err != nil {
		return nil, err
	}
	return storagebrokerv0alphapb.NewStorageBrokerServiceClient(conn), nil
}

func getAppRegistryClient() (appregistryv0alphapb.AppRegistryServiceClient, error) {
	conn, err := getConn()
	if err != nil {
		return nil, err
	}
	return appregistryv0alphapb.NewAppRegistryServiceClient(conn), nil
}

func getStorageProviderClient(host string) (storageproviderv0alphapb.StorageProviderServiceClient, error) {
	conn, err := getConnToHost(host)
	if err != nil {
		return nil, err
	}
	return storageproviderv0alphapb.NewStorageProviderServiceClient(conn), nil
}

func getAuthClient() (authv0alphapb.AuthServiceClient, error) {
	conn, err := getConn()
	if err != nil {
		return nil, err
	}
	return authv0alphapb.NewAuthServiceClient(conn), nil
}

func getConn() (*grpc.ClientConn, error) {
	return grpc.Dial(conf.Host, grpc.WithInsecure())
}

func getConnToHost(host string) (*grpc.ClientConn, error) {
	return grpc.Dial(host, grpc.WithInsecure())
}

func formatError(status *rpcpb.Status) error {
	switch status.Code {
	case rpcpb.Code_CODE_NOT_FOUND:
		return errors.New("error: not found")

	default:
		return errors.New(fmt.Sprintf("apierror: code=%v msg=%s", status.Code, status.Message))
	}
}
