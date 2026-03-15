// Package config provides application configuration loading, logger setup,
// and OpenTelemetry tracer initialization for the Apex Upload Platform.
package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config holds all application configuration values loaded from config.yaml
// and/or environment variables. Environment variables take precedence over
// file-based values when both are present.
type Config struct {

	// AppEnv indicates the application environment (e.g., "local", "production").
	AppEnv string `mapstructure:"APP_ENV"`
	// HttpPort is the TCP port the HTTP server listens on (default "8080").
	HttpPort string `mapstructure:"HTTP_PORT"`
	// GCPProjectID is the Google Cloud project ID used for GCS, Firestore, and Secret Manager.
	GCPProjectID string `mapstructure:"GCP_PROJECT_ID"`
	// GCSBucket is the GCS bucket where uploaded objects are stored.
	GCSBucket string `mapstructure:"GCS_BUCKET"`
	// OTEL_SERVICE_NAME is the logical service name reported to the OpenTelemetry collector.
	OTEL_SERVICE_NAME string `mapstructure:"OTEL_SERVICE_NAME"`

	MaxFileSizeBytes    int64    `mapstructure:"MAX_FILE_SIZE_BYTES"`
	MinFileSizeBytes    int64    `mapstructure:"MIN_FILE_SIZE_BYTES"`
	MagicByteHeaderSize int64    `mapstructure:"MAGIC_BYTE_HEADER_SIZE"`
	AllowedVideoFormats []string `mapstructure:"ALLOWED_VIDEO_FORMATS"`

	ProjectRegion        string `mapstructure:"GCP_PROJECT_REGION"`
	ServiceAccountEmail  string `mapstructure:"INVOKER_SERVICE_ACCOUNT_EMAIL"`
	TranscoderServiceUrl string `mapstructure:"TRANSCODER_SERVICE_URL"`
	CloudTasksQueuePath  string `mapstructure:"CLOUD_TASKS_QUEUE_PATH"`
	CloudTasksQueueName  string `mapstructure:"CLOUD_TASKS_QUEUE_NAME"`

	PubSubSubscriptionID    string `mapstructure:"PUBSUB_SUBSCRIPTION_ID"`
	MaxOutstandingMessages  int    `mapstructure:"MAX_OUTSTANDING_MESSAGES"`
	FirestoreCollectionName string `mapstructure:"FIRESTORE_COLLECTION_NAME"`
	FirestoreDatabaseID     string `mapstructure:"FIRESTORE_DATABASE_ID"`
}

// LoadConfig reads configuration from a YAML file at the given path and merges
// it with environment variables. Sensible defaults are provided for local
// development. Returns an error if the config file exists but cannot be parsed,
// or if the values cannot be unmarshalled into the Config struct.
func LoadConfig(path string) (*Config, error) {

	// Set sensible defaults for local development.
	viper.SetDefault("APP_ENV", "development")
	viper.SetDefault("GCP_PROJECT_REGION", "us-central1")
	viper.SetDefault("HTTP_PORT", ":8080")
	viper.SetDefault("GCP_PROJECT_ID", "amith-testing")
	viper.SetDefault("GCS_BUCKET", "")
	viper.SetDefault("OTEL_SERVICE_NAME", "prj-apex-upload-platform")
	viper.SetDefault("OTEL_EXPORTER_OTLP_HEADERS", "x-goog-user-project=amith-testing")
	viper.SetDefault("MAX_FILE_SIZE_BYTES", 500_000_000) // 500 MB
	viper.SetDefault("MIN_FILE_SIZE_BYTES", 1_000_000)   // 1 MB
	viper.SetDefault("MAGIC_BYTE_HEADER_SIZE", 12)       // Read first 12 bytes for magic byte validation
	viper.SetDefault("ALLOWED_VIDEO_FORMATS", []string{"mp4", "avi", "mkv"})
	viper.SetDefault("CLOUD_TASKS_QUEUE_PATH", "")
	viper.SetDefault("INVOKER_SERVICE_ACCOUNT_EMAIL", "")
	viper.SetDefault("TRANSCODER_SERVICE_URL", "")
	viper.SetDefault("CLOUD_TASKS_QUEUE_NAME", "")
	viper.SetDefault("PUBSUB_SUBSCRIPTION_ID", "")
	viper.SetDefault("MAX_OUTSTANDING_MESSAGES", 10)
	viper.SetDefault("FIRESTORE_COLLECTION_NAME", "videos")
	viper.SetDefault("FIRESTORE_DATABASE_ID", "apex-firestore-db")

	viper.AddConfigPath(path)
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	// Bind environment variables; dots in keys become underscores (e.g., GCS.BUCKET -> GCS_BUCKET).
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Read the config file; ignore "file not found" since env vars may supply all values.
	err := viper.ReadInConfig()
	if _, ok := err.(viper.ConfigFileNotFoundError); err != nil && !ok {
		return nil, fmt.Errorf("fatal error config file: %w", err)
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("unable to decode into struct, %w", err)
	}

	return &config, nil
}
