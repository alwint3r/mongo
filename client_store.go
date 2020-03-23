package mongo

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/oauth2.v3"
	"gopkg.in/oauth2.v3/models"
)

// ClientConfig client configuration parameters
type ClientConfig struct {
	// store clients data collection name(The default is oauth2_clients)
	ClientsCName string
}

// NewDefaultClientConfig create a default client configuration
func NewDefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		ClientsCName: "oauth2_clients",
	}
}

// NewClientStore create a client store instance based on mongodb
func NewClientStore(cfg *Config, ccfgs ...*ClientConfig) *ClientStore {
	clientOptions := options.Client().ApplyURI(cfg.URL)
	client, err := mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		panic(err)
	}

	return NewClientStoreWithMongoClient(client, cfg.DB, ccfgs...)
}

// NewClientStoreWithMongoClient create a client soter instance based on mongodb
func NewClientStoreWithMongoClient(client *mongo.Client, dbName string, ccfgs ...*ClientConfig) *ClientStore {
	store := &ClientStore{
		dbName: dbName,
		ccfg:   NewDefaultClientConfig(),
		client: client,
	}

	if len(ccfgs) > 0 {
		store.ccfg = ccfgs[0]
	}

	return store
}

// ClientStore MongoDB storage for OAuth 2.0
type ClientStore struct {
	ccfg   *ClientConfig
	dbName string
	client *mongo.Client
}

// Close close the mongodb connection
func (cs *ClientStore) Close() {
	cs.client.Disconnect(context.TODO())
}

func (cs *ClientStore) c(name string) *mongo.Collection {
	db := cs.client.Database(cs.dbName)
	return db.Collection(name)
}

func (cs *ClientStore) cHandler(name string, handler func(c *mongo.Collection)) {
	handler(cs.client.Database(cs.dbName).Collection(name))
}

// Set set client information
func (cs *ClientStore) Set(info oauth2.ClientInfo) (err error) {
	cs.cHandler(cs.ccfg.ClientsCName, func(c *mongo.Collection) {
		entity := &client{
			ID:     info.GetID(),
			Secret: info.GetSecret(),
			Domain: info.GetDomain(),
			UserID: info.GetUserID(),
		}

		if _, cerr := c.InsertOne(context.TODO(), entity); cerr != nil {
			err = cerr
			return
		}
	})

	return
}

// GetByID according to the ID for the client information
func (cs *ClientStore) GetByID(id string) (info oauth2.ClientInfo, err error) {
	cs.cHandler(cs.ccfg.ClientsCName, func(c *mongo.Collection) {
		entity := new(client)
		filter := bson.M{"_id": id}

		if cerr := c.FindOne(context.TODO(), filter).Decode(entity); cerr != nil {
			err = cerr
			return
		}

		info = &models.Client{
			ID:     entity.ID,
			Secret: entity.Secret,
			Domain: entity.Domain,
			UserID: entity.UserID,
		}
	})

	return
}

// RemoveByID use the client id to delete the client information
func (cs *ClientStore) RemoveByID(id string) (err error) {
	cs.cHandler(cs.ccfg.ClientsCName, func(c *mongo.Collection) {
		filter := bson.M{"_id": id}

		result, cerr := c.DeleteOne(context.TODO(), filter)
		if cerr != nil {
			err = cerr
			return
		}

		if result.DeletedCount == 0 {
			err = errors.New("Unknown client")
			return
		}
	})

	return
}

type client struct {
	ID     string `bson:"_id"`
	Secret string `bson:"secret"`
	Domain string `bson:"domain"`
	UserID string `bson:"userid"`
}
