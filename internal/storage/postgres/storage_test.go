package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/dipdup-io/starknet-metadata/internal/storage"
	"github.com/dipdup-net/go-lib/config"
	"github.com/dipdup-net/go-lib/database"
	"github.com/go-testfixtures/testfixtures/v3"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/suite"
)

type TestSuite struct {
	suite.Suite
	psqlContainer *database.PostgreSQLContainer
	storage       Storage
}

// SetupSuite -
func (s *TestSuite) SetupSuite() {
	ctx, ctxCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer ctxCancel()

	psqlContainer, err := database.NewPostgreSQLContainer(ctx, database.PostgreSQLContainerConfig{
		User:     "user",
		Password: "password",
		Database: "db_test",
		Port:     5432,
		Image:    "postgres:15",
	})
	s.Require().NoError(err)
	s.psqlContainer = psqlContainer

	s.storage, err = Create(ctx, config.Database{
		Kind:     config.DBKindPostgres,
		User:     s.psqlContainer.Config.User,
		Database: s.psqlContainer.Config.Database,
		Password: s.psqlContainer.Config.Password,
		Host:     s.psqlContainer.Config.Host,
		Port:     s.psqlContainer.MappedPort().Int(),
	})
	s.Require().NoError(err)

	db, err := sql.Open("postgres", s.psqlContainer.GetDSN())
	s.Require().NoError(err)

	fixtures, err := testfixtures.New(
		testfixtures.Database(db),
		testfixtures.Dialect("postgres"),
		testfixtures.Directory("fixtures"),
	)
	s.Require().NoError(err)
	s.Require().NoError(fixtures.Load())
	s.Require().NoError(db.Close())
}

func (s *TestSuite) TearDownSuite() {
	ctx, ctxCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer ctxCancel()

	s.Require().NoError(s.storage.Close())
	s.Require().NoError(s.psqlContainer.Terminate(ctx))
}

func (s *TestSuite) TestTokenMetadataGetByStatus() {
	for _, status := range []storage.Status{
		storage.StatusFailed,
		storage.StatusFilled,
		storage.StatusNew,
		storage.StatusSuccess,
	} {
		ctx, ctxCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ctxCancel()
		response, err := s.storage.TokenMetadata.GetByStatus(ctx, status, 1, 0, 0, 0)
		s.Require().NoError(err)

		s.Require().Len(response, 1)
		s.Require().Equal(response[0].Status, status)
		s.Require().Len(response[0].Contract.Hash, 32)
	}
}

func (s *TestSuite) TestStateByName() {

	ctx, ctxCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer ctxCancel()

	response, err := s.storage.State.ByName(ctx, "test")
	s.Require().NoError(err)
	s.Require().EqualValues(1, response.ID)
	s.Require().EqualValues(100, response.LastHeight)

	_, err = s.storage.State.ByName(ctx, "unknown")
	s.Require().Error(err)
}

func (s *TestSuite) TestTxSaveAddress() {
	ctx, ctxCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer ctxCancel()

	tx, err := BeginTransaction(ctx, s.storage.Transactable)
	s.Require().NoError(err)
	defer tx.Close(ctx)

	err = tx.SaveAddress(ctx, &storage.Address{
		ID:   100,
		Hash: []byte{0, 1, 2, 3, 4},
	})
	s.Require().NoError(err)

	err = tx.Flush(ctx)
	s.Require().NoError(err)

	response, err := s.storage.Address.GetByID(ctx, 100)
	s.Require().NoError(err)
	s.Require().EqualValues(100, response.ID)
	s.Require().Equal([]byte{0, 1, 2, 3, 4}, response.Hash)
}

func TestSuite_Run(t *testing.T) {
	suite.Run(t, new(TestSuite))
}
