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

type kafkaACLCatalog struct {
	SchemaVersion int `json:"schema_version"`
	TopicBindings []struct {
		Topic string `json:"topic"`
	} `json:"topic_bindings"`
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
		case "producer_candidate_default_off", "consumer_candidate_default_off":
			require.NotEmpty(t, topic.Producers, topic.Name)
			require.NotEmpty(t, topic.Consumers, topic.Name)
		case "producer_only":
			require.NotEmpty(t, topic.Producers, topic.Name)
			require.Empty(t, topic.Consumers, topic.Name)
		case "consumer_only":
			require.Empty(t, topic.Producers, topic.Name)
			require.NotEmpty(t, topic.Consumers, topic.Name)
		case "reserved":
			// 合同冻结、topic 就绪,但生产/消费接线尚未落地(如实登记,禁止编造代码路径)
			require.Empty(t, topic.Producers, topic.Name)
			require.Empty(t, topic.Consumers, topic.Name)
		case "dlq":
			require.NotEmpty(t, topic.Producers, topic.Name)
		default:
			t.Fatalf("topic %s has unsupported readiness %q", topic.Name, topic.Readiness)
		}
	}

	aclPayload, err := os.ReadFile(filepath.Join(root, "contracts", "events", "kafka-acl-catalog.v1.json"))
	require.NoError(t, err)
	var aclCatalog kafkaACLCatalog
	require.NoError(t, json.Unmarshal(aclPayload, &aclCatalog))
	require.Equal(t, 1, aclCatalog.SchemaVersion)
	aclTopics := make(map[string]struct{}, len(aclCatalog.TopicBindings))
	for _, binding := range aclCatalog.TopicBindings {
		require.NotEmpty(t, binding.Topic)
		_, duplicate := aclTopics[binding.Topic]
		require.False(t, duplicate, "duplicate Kafka ACL binding %s", binding.Topic)
		aclTopics[binding.Topic] = struct{}{}
	}
	require.Equal(t, seen, aclTopics, "topic contracts and least-privilege ACL bindings must have the same exact topic set")
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
	require.Positive(t, counts["json-schema"])
	require.Positive(t, counts["protobuf"])
	require.Equal(t, len(catalog.Topics), counts["json-schema"]+counts["protobuf"])
}
