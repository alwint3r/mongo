package mongo

import (
	"context"
	"encoding/json"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"gopkg.in/oauth2.v3"
	"gopkg.in/oauth2.v3/models"
)

// TokenConfig token configuration parameters
type TokenConfig struct {
	// store txn collection name(The default is oauth2)
	TxnCName string
	// store token based data collection name(The default is oauth2_basic)
	BasicCName string
	// store access token data collection name(The default is oauth2_access)
	AccessCName string
	// store refresh token data collection name(The default is oauth2_refresh)
	RefreshCName string
}

// NewDefaultTokenConfig create a default token configuration
func NewDefaultTokenConfig() *TokenConfig {
	return &TokenConfig{
		TxnCName:     "oauth2_txn",
		BasicCName:   "oauth2_basic",
		AccessCName:  "oauth2_access",
		RefreshCName: "oauth2_refresh",
	}
}

// NewTokenStore create a token store instance based on mongodb
func NewTokenStore(cfg *Config, tcfgs ...*TokenConfig) (store *TokenStore) {
	clientOptions := options.Client().ApplyURI(cfg.URL)
	client, err := mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		panic(err)
	}
	if err != nil {
		panic(err)
	}

	return NewTokenStoreWithMongoClient(client, cfg.DB, tcfgs...)
}

// NewTokenStoreWithMongoClient create a token store instance based on mongodb
func NewTokenStoreWithMongoClient(client *mongo.Client, dbName string, tcfgs ...*TokenConfig) (store *TokenStore) {
	tokenStore := &TokenStore{
		dbName: dbName,
		tcfg:   NewDefaultTokenConfig(),
		client: client,
	}

	if len(tcfgs) > 0 {
		tokenStore.tcfg = tcfgs[0]
	}

	var expireAfter int32
	expireAfter = 1

	tokenStore.c(tokenStore.tcfg.BasicCName).Indexes().CreateOne(context.TODO(), mongo.IndexModel{
		Keys: []string{"ExpiredAt"},
		Options: &options.IndexOptions{
			ExpireAfterSeconds: &expireAfter,
		},
	})

	tokenStore.c(tokenStore.tcfg.AccessCName).Indexes().CreateOne(context.TODO(), mongo.IndexModel{
		Keys: []string{"ExpiredAt"},
		Options: &options.IndexOptions{
			ExpireAfterSeconds: &expireAfter,
		},
	})

	tokenStore.c(tokenStore.tcfg.RefreshCName).Indexes().CreateOne(context.TODO(), mongo.IndexModel{
		Keys: []string{"ExpiredAt"},
		Options: &options.IndexOptions{
			ExpireAfterSeconds: &expireAfter,
		},
	})

	store = tokenStore

	return
}

// TokenStore MongoDB storage for OAuth 2.0
type TokenStore struct {
	tcfg   *TokenConfig
	dbName string
	client *mongo.Client
}

// Close close the mongo session
func (ts *TokenStore) Close() {
	ts.client.Disconnect(context.TODO())
}

func (ts *TokenStore) c(name string) *mongo.Collection {
	db := ts.client.Database(ts.dbName)
	return db.Collection(name)
}

func (ts *TokenStore) cHandler(name string, handler func(mongo.Session, *mongo.Collection)) {
	session, err := ts.client.StartSession(nil)
	if err != nil {
		return
	}

	defer session.EndSession(context.TODO())

	handler(session, ts.client.Database(ts.dbName).Collection(name))
	return
}

// Create create and store the new token information
func (ts *TokenStore) Create(info oauth2.TokenInfo) (err error) {
	jsonData, err := json.Marshal(info)
	if err != nil {
		return
	}

	if code := info.GetCode(); code != "" {
		ts.cHandler(ts.tcfg.BasicCName, func(sess mongo.Session, c *mongo.Collection) {
			_, err = c.InsertOne(context.TODO(), basicData{
				ID:        code,
				Data:      jsonData,
				ExpiredAt: info.GetCodeCreateAt().Add(info.GetCodeExpiresIn()),
			})
		})

		return
	}

	aexp := info.GetAccessCreateAt().Add(info.GetAccessExpiresIn())
	rexp := aexp

	if refresh := info.GetRefresh(); refresh != "" {
		rexp = info.GetRefreshCreateAt().Add(info.GetRefreshExpiresIn())
		if aexp.Second() > rexp.Second() {
			aexp = rexp
		}
	}

	id := primitive.NewObjectID()
	txnOpts := options.Transaction().SetReadPreference(readpref.PrimaryPreferred())
	ts.cHandler(ts.tcfg.TxnCName, func(sess mongo.Session, c *mongo.Collection) {
		_, txErr := sess.WithTransaction(context.TODO(), func(sessionContext mongo.SessionContext) (interface{}, error) {
			basicCollection := ts.c(ts.tcfg.BasicCName)
			_, insertErr := basicCollection.InsertOne(sessionContext, basicData{
				ID:        id.Hex(),
				Data:      jsonData,
				ExpiredAt: rexp,
			})

			if insertErr != nil {
				return nil, insertErr
			}

			accessCollection := ts.c(ts.tcfg.AccessCName)
			_, insertErr = accessCollection.InsertOne(sessionContext, tokenData{
				ID:        info.GetAccess(),
				BasicID:   id.Hex(),
				ExpiredAt: aexp,
			})

			if insertErr != nil {
				return nil, insertErr
			}

			refreshToken := info.GetRefresh()

			if refreshToken != "" {
				refreshCollection := ts.c(ts.tcfg.RefreshCName)
				_, insertErr = refreshCollection.InsertOne(sessionContext, tokenData{
					ID:        refreshToken,
					BasicID:   id.Hex(),
					ExpiredAt: rexp,
				})

				if insertErr != nil {
					return nil, insertErr
				}
			}

			return nil, nil
		}, txnOpts)

		if txErr != nil {
			err = txErr
			return
		}
	})

	return
}

// RemoveByCode use the authorization code to delete the token information
func (ts *TokenStore) RemoveByCode(code string) (err error) {
	ts.cHandler(ts.tcfg.BasicCName, func(sess mongo.Session, c *mongo.Collection) {
		result, derr := c.DeleteOne(context.TODO(), bson.M{"_id": code})
		if derr != nil {
			if result.DeletedCount == 0 {
				return
			}

			err = derr
		}
	})

	return
}

// RemoveByAccess use the access token to delete the token information
func (ts *TokenStore) RemoveByAccess(access string) (err error) {
	ts.cHandler(ts.tcfg.AccessCName, func(sess mongo.Session, c *mongo.Collection) {
		result, derr := c.DeleteOne(context.TODO(), bson.M{"_id": access})
		if derr != nil {
			if result.DeletedCount == 0 {
				return
			}

			err = derr
		}
	})

	return
}

// RemoveByRefresh use the refresh token to delete the token information
func (ts *TokenStore) RemoveByRefresh(refresh string) (err error) {
	ts.cHandler(ts.tcfg.RefreshCName, func(sess mongo.Session, c *mongo.Collection) {
		result, derr := c.DeleteOne(context.TODO(), bson.M{"_id": refresh})
		if derr != nil {
			if result.DeletedCount == 0 {
				return
			}

			err = derr
		}
	})

	return
}

func (ts *TokenStore) getData(basicID string) (tokenInfo oauth2.TokenInfo, err error) {
	ts.cHandler(ts.tcfg.BasicCName, func(sess mongo.Session, c *mongo.Collection) {
		var data basicData
		rerr := c.FindOne(context.TODO(), bson.M{"_id": basicID}).Decode(&data)
		if rerr != nil {
			if rerr == mongo.ErrNoDocuments {
				return
			}

			err = rerr
			return
		}

		var tokenModel models.Token
		err = json.Unmarshal(data.Data, &tokenModel)
		if err != nil {
			return
		}

		tokenInfo = &tokenModel
	})

	return
}

func (ts *TokenStore) getBasicID(collectionName, token string) (basicID string, err error) {
	ts.cHandler(collectionName, func(sess mongo.Session, c *mongo.Collection) {
		var data tokenData
		rerr := c.FindOne(context.TODO(), bson.M{"_id": token}).Decode(&data)
		if rerr != nil {
			if rerr == mongo.ErrNoDocuments {
				return
			}

			err = rerr
			return
		}

		basicID = data.BasicID
	})

	return
}

// GetByCode use the authorization code for token information data
func (ts *TokenStore) GetByCode(code string) (tokenInfo oauth2.TokenInfo, err error) {
	tokenInfo, err = ts.getData(code)
	return
}

// GetByAccess use the access token for token information data
func (ts *TokenStore) GetByAccess(access string) (tokenInfo oauth2.TokenInfo, err error) {
	basicID, err := ts.getBasicID(ts.tcfg.AccessCName, access)
	if err != nil && basicID == "" {
		return
	}
	tokenInfo, err = ts.getData(basicID)
	return
}

// GetByRefresh use the refresh token for token information data
func (ts *TokenStore) GetByRefresh(refresh string) (tokenInfo oauth2.TokenInfo, err error) {
	basicID, err := ts.getBasicID(ts.tcfg.RefreshCName, refresh)
	if err != nil && basicID == "" {
		return
	}
	tokenInfo, err = ts.getData(basicID)
	return
}

type basicData struct {
	ID        string    `bson:"_id"`
	Data      []byte    `bson:"Data"`
	ExpiredAt time.Time `bson:"ExpiredAt"`
}

type tokenData struct {
	ID        string    `bson:"_id"`
	BasicID   string    `bson:"BasicID"`
	ExpiredAt time.Time `bson:"ExpiredAt"`
}
