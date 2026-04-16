package data

import (
	"context"
	"fmt"
	"testing"
	"time"
	"user/ent"
	"user/ent/user"

	"entgo.io/ent/dialect/sql"
)

func TestDatabaseOperation(t *testing.T) {
	client, err := sql.Open("postgres", fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=disable",
		"simple-net.dynv6.net",
		"dylan",
		"yongheng290..",
		"cloud_storage",
		5444))
	if err != nil {
		t.Errorf("failed to open database: %s", err)
	}
	db := client.DB()
	db.SetMaxIdleConns(50)
	db.SetConnMaxLifetime(time.Second * 30)
	driverOpt := ent.Driver(client)
	c := ent.NewClient(driverOpt)
	uc := NewUserClient(c)

	for i := range 100 {
		t.Logf("%d轮\n", i)
		args := &NewUserArgs{
			Email:         fmt.Sprintf("user-%d", i),
			PlainPassword: "",
			Status:        user.StatusActive,
			GroupID:       3,
		}
		_, err := uc.Create(context.Background(), args)
		if err != nil {
			t.Errorf("failed to create user: %s", err)
		}
	}
}
