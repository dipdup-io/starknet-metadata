package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/dipdup-io/starknet-metadata/internal/storage"
	"github.com/dipdup-net/go-lib/config"
	"github.com/docker/go-connections/nat"
	"github.com/go-testfixtures/testfixtures/v3"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

type (
	PostgreSQLContainer struct {
		testcontainers.Container
		cfg PostgreSQLContainerConfig
	}

	PostgreSQLContainerConfig struct {
		User       string
		Password   string
		Database   string
		MappedPort nat.Port
		Host       string
	}

	TestSuite struct {
		suite.Suite
		psqlContainer *PostgreSQLContainer
		storage       Storage
	}
)

// NewPostgreSQLContainer -
func NewPostgreSQLContainer(ctx context.Context, cfg PostgreSQLContainerConfig) (*PostgreSQLContainer, error) {
	container, err := postgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:15"),
		postgres.WithDatabase(cfg.Database),
		postgres.WithUsername(cfg.User),
		postgres.WithPassword(cfg.Password),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(5*time.Second)),
	)
	if err != nil {
		return nil, err
	}

	host, err := container.Host(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting host for: %w", err)
	}

	mappedPort, err := container.MappedPort(ctx, nat.Port("5432/tcp"))
	if err != nil {
		return nil, fmt.Errorf("getting mapped port for 5432/tcp: %w", err)
	}
	cfg.MappedPort = mappedPort
	cfg.Host = host

	return &PostgreSQLContainer{
		Container: container,
		cfg:       cfg,
	}, nil
}

// GetDSN -
func (c PostgreSQLContainer) GetDSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", c.cfg.User, c.cfg.Password, c.cfg.Host, c.cfg.MappedPort.Port(), c.cfg.Database)
}

// SetupSuite -
func (s *TestSuite) SetupSuite() {
	ctx, ctxCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer ctxCancel()

	psqlContainer, err := NewPostgreSQLContainer(ctx, PostgreSQLContainerConfig{
		User:     "user",
		Password: "password",
		Database: "starknet_metadata_test",
	})
	s.Require().NoError(err)
	s.psqlContainer = psqlContainer

	s.storage, err = Create(ctx, config.Database{
		Kind:     config.DBKindPostgres,
		User:     s.psqlContainer.cfg.User,
		Database: s.psqlContainer.cfg.Database,
		Password: s.psqlContainer.cfg.Password,
		Host:     s.psqlContainer.cfg.Host,
		Port:     s.psqlContainer.cfg.MappedPort.Int(),
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
}

func (s *TestSuite) TearDownSuite() {
	ctx, ctxCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer ctxCancel()

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
	}
}

func TestSuite_Run(t *testing.T) {
	suite.Run(t, new(TestSuite))
}
