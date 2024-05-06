package database

import (
	"context"
	"errors"
	"github.com/romitou/previsix/config"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var client *mongo.Client

func Connect() error {
	uri := config.Get().Database.URI
	if uri == "" {
		return errors.New("mongodb uri is empty")
	}

	var err error
	client, err = mongo.Connect(context.TODO(), options.Client().ApplyURI(uri))
	if err != nil {
		return err
	}

	defer client.Disconnect(context.TODO())

	err = client.Ping(context.TODO(), nil)

	return err
}

func Get() *mongo.Client {
	return client
}
