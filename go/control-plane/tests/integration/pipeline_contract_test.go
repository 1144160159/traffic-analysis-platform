package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

type kafkaTopicCatalog struct {
	SchemaVersion int `json:"schema_version"`
	Topics        []struct {
		Name        string   `json:"name"`
		MessageType string   `json:"message_type"`
		Readiness   string   `json:"readiness"`
		Producers   []string `json:"producers"`
		Consumers   []string `json:"consumers"`
		Schema      struct {
			Kind       string `json:"kind"`
			Path       string `json:"path"`
			Message    string `json:"message"`
			Definition string `json:"definition"`
		} `json:"schema"`
	} `json:"topics"`
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(directory, "contracts", "events")); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		require.NotEqual(t, directory, parent, "repository root not found from %s", directory)
		directory = parent
	}
}

func loadKafkaTopicCatalog(t *testing.T) (string, kafkaTopicCatalog) {
	t.Helper()
	root := repositoryRoot(t)
	payload, err := os.ReadFile(filepath.Join(root, "contracts", "events", "kafka-topic-catalog.v1.json"))
	require.NoError(t, err)
	var catalog kafkaTopicCatalog
	require.NoError(t, json.Unmarshal(payload, &catalog))
	return root, catalog
}

func TestPipelineContractCompleteMapping(t *testing.T) {
	root, catalog := loadKafkaTopicCatalog(t)
	require.Equal(t, 1, catalog.SchemaVersion)
	require.Len(t, catalog.Topics, 35)

	seen := map[string]struct{}{}
	for _, topic := range catalog.Topics {
		require.NotEmpty(t, topic.Name)
		require.NotEmpty(t, topic.MessageType, topic.Name)
		require.NotEmpty(t, topic.Schema.Kind, topic.Name)
		require.FileExists(t, filepath.Join(root, topic.Schema.Path), topic.Name)
		_, duplicate := seen[topic.Name]
		require.False(t, duplicate, "duplicate Kafka topic %s", topic.Name)
		seen[topic.Name] = struct{}{}

		switch topic.Readiness {
		case "active":
			require.NotEmpty(t, topic.Producers, topic.Name)
			require.NotEmpty(t, topic.Consumers, topic.Name)
		case "producer_only":
			require.NotEmpty(t, topic.Producers, topic.Name)
			require.Empty(t, topic.Consumers, topic.Name)
		case "consumer_only":
			require.Empty(t, topic.Producers, topic.Name)
			require.NotEmpty(t, topic.Consumers, topic.Name)
		case "dlq":
			require.NotEmpty(t, topic.Producers, topic.Name)
		default:
			t.Fatalf("topic %s has unsupported readiness %q", topic.Name, topic.Readiness)
		}
	}
}

func TestPipelineContractSchemaKindsRemainExplicit(t *testing.T) {
	_, catalog := loadKafkaTopicCatalog(t)
	counts := map[string]int{}
	for _, topic := range catalog.Topics {
		counts[topic.Schema.Kind]++
		if topic.Schema.Kind == "protobuf" {
			require.NotEmpty(t, topic.Schema.Message, topic.Name)
			require.Empty(t, topic.Schema.Definition, topic.Name)
		} else {
			require.Equal(t, "json-schema", topic.Schema.Kind, topic.Name)
			require.NotEmpty(t, topic.Schema.Definition, topic.Name)
			require.Empty(t, topic.Schema.Message, topic.Name)
		}
	}
	require.Equal(t, map[string]int{"json-schema": 25, "protobuf": 10}, counts)
}
