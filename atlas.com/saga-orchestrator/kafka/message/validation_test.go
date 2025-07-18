package message

import (
	"encoding/json"
	"testing"

	"github.com/Chronicle20/atlas-kafka/producer"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageValidation(t *testing.T) {
	t.Run("valid message key format", func(t *testing.T) {
		characterId := 12345
		key := producer.CreateKey(characterId)

		// Key should be non-empty byte slice
		assert.NotEmpty(t, key)
		assert.IsType(t, []byte{}, key)
	})

	t.Run("message key consistency", func(t *testing.T) {
		characterId := 12345

		// Same character ID should generate same key
		key1 := producer.CreateKey(characterId)
		key2 := producer.CreateKey(characterId)
		assert.Equal(t, key1, key2)

		// Different character IDs should generate different keys
		key3 := producer.CreateKey(67890)
		assert.NotEqual(t, key1, key3)
	})

	t.Run("message value is serializable", func(t *testing.T) {
		// Test with various message types
		testCases := []struct {
			name    string
			message interface{}
		}{
			{
				name:    "string message",
				message: "test message",
			},
			{
				name: "json object",
				message: map[string]interface{}{
					"type": "test",
					"data": "value",
				},
			},
			{
				name: "struct message",
				message: struct {
					ID   string `json:"id"`
					Type string `json:"type"`
				}{
					ID:   uuid.New().String(),
					Type: "test",
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Should be able to serialize to JSON
				data, err := json.Marshal(tc.message)
				require.NoError(t, err)
				assert.NotEmpty(t, data)

				// Should be able to deserialize back
				var result interface{}
				err = json.Unmarshal(data, &result)
				require.NoError(t, err)
				assert.NotNil(t, result)
			})
		}
	})

	t.Run("transaction ID validation", func(t *testing.T) {
		testCases := []struct {
			name          string
			transactionId string
			expectValid   bool
		}{
			{
				name:          "valid UUID",
				transactionId: uuid.New().String(),
				expectValid:   true,
			},
			{
				name:          "empty string",
				transactionId: "",
				expectValid:   false,
			},
			{
				name:          "invalid UUID format",
				transactionId: "not-a-uuid",
				expectValid:   false,
			},
			{
				name:          "malformed UUID",
				transactionId: "12345678-1234-1234-1234-123456789012x",
				expectValid:   false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := uuid.Parse(tc.transactionId)
				if tc.expectValid {
					assert.NoError(t, err)
				} else {
					assert.Error(t, err)
				}
			})
		}
	})

	t.Run("character ID validation", func(t *testing.T) {
		testCases := []struct {
			name        string
			characterId uint32
			expectValid bool
		}{
			{
				name:        "valid character ID",
				characterId: 12345,
				expectValid: true,
			},
			{
				name:        "zero character ID",
				characterId: 0,
				expectValid: false,
			},
			{
				name:        "max uint32",
				characterId: ^uint32(0),
				expectValid: true,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				if tc.expectValid {
					assert.Greater(t, tc.characterId, uint32(0))
				} else {
					assert.Equal(t, uint32(0), tc.characterId)
				}
			})
		}
	})

	t.Run("message buffer validation", func(t *testing.T) {
		buffer := NewBuffer()
		assert.NotNil(t, buffer)

		// Empty buffer should return empty map
		messages := buffer.GetAll()
		assert.NotNil(t, messages)
		assert.Empty(t, messages)

		// Buffer should handle multiple topics
		topic1 := "test-topic-1"
		topic2 := "test-topic-2"

		message1 := kafka.Message{
			Key:   []byte("key1"),
			Value: []byte("value1"),
		}
		message2 := kafka.Message{
			Key:   []byte("key2"),
			Value: []byte("value2"),
		}

		// Create providers for the messages
		provider1 := func() ([]kafka.Message, error) { return []kafka.Message{message1}, nil }
		provider2 := func() ([]kafka.Message, error) { return []kafka.Message{message2}, nil }

		err := buffer.Put(topic1, provider1)
		assert.NoError(t, err)
		err = buffer.Put(topic2, provider2)
		assert.NoError(t, err)

		allMessages := buffer.GetAll()
		assert.Len(t, allMessages, 2)
		assert.Contains(t, allMessages, topic1)
		assert.Contains(t, allMessages, topic2)
		assert.Equal(t, []kafka.Message{message1}, allMessages[topic1])
		assert.Equal(t, []kafka.Message{message2}, allMessages[topic2])
	})

	t.Run("message buffer accumulation", func(t *testing.T) {
		buffer := NewBuffer()
		topic := "test-topic"

		message1 := kafka.Message{Key: []byte("key1"), Value: []byte("value1")}
		message2 := kafka.Message{Key: []byte("key2"), Value: []byte("value2")}
		message3 := kafka.Message{Key: []byte("key3"), Value: []byte("value3")}

		// Create providers for the messages
		provider1 := func() ([]kafka.Message, error) { return []kafka.Message{message1}, nil }
		provider2 := func() ([]kafka.Message, error) { return []kafka.Message{message2}, nil }
		provider3 := func() ([]kafka.Message, error) { return []kafka.Message{message3}, nil }

		err := buffer.Put(topic, provider1)
		assert.NoError(t, err)
		err = buffer.Put(topic, provider2)
		assert.NoError(t, err)
		err = buffer.Put(topic, provider3)
		assert.NoError(t, err)

		messages := buffer.GetAll()
		assert.Len(t, messages, 1)
		assert.Contains(t, messages, topic)
		assert.Len(t, messages[topic], 3)

		// Messages should be in order
		assert.Equal(t, message1, messages[topic][0])
		assert.Equal(t, message2, messages[topic][1])
		assert.Equal(t, message3, messages[topic][2])
	})
}

func TestMessageBufferErrorHandling(t *testing.T) {
	t.Run("buffer with nil messages", func(t *testing.T) {
		buffer := NewBuffer()
		topic := "test-topic"

		// Put nil message - should handle gracefully
		nilMessage := kafka.Message{
			Key:   []byte("key1"),
			Value: nil,
		}

		provider := func() ([]kafka.Message, error) { return []kafka.Message{nilMessage}, nil }
		err := buffer.Put(topic, provider)
		assert.NoError(t, err)

		messages := buffer.GetAll()
		assert.Len(t, messages, 1)
		assert.Contains(t, messages, topic)
		assert.Len(t, messages[topic], 1)
		assert.Equal(t, nilMessage, messages[topic][0])
	})

	t.Run("buffer with empty key", func(t *testing.T) {
		buffer := NewBuffer()
		topic := "test-topic"

		emptyKeyMessage := kafka.Message{
			Key:   []byte(""),
			Value: []byte("value"),
		}

		provider := func() ([]kafka.Message, error) { return []kafka.Message{emptyKeyMessage}, nil }
		err := buffer.Put(topic, provider)
		assert.NoError(t, err)

		messages := buffer.GetAll()
		assert.Len(t, messages, 1)
		assert.Contains(t, messages, topic)
		assert.Len(t, messages[topic], 1)
		assert.Equal(t, emptyKeyMessage, messages[topic][0])
	})

	t.Run("buffer with empty topic", func(t *testing.T) {
		buffer := NewBuffer()
		emptyTopic := ""

		message := kafka.Message{
			Key:   []byte("key1"),
			Value: []byte("value1"),
		}

		provider := func() ([]kafka.Message, error) { return []kafka.Message{message}, nil }
		err := buffer.Put(emptyTopic, provider)
		assert.NoError(t, err)

		messages := buffer.GetAll()
		assert.Len(t, messages, 1)
		assert.Contains(t, messages, emptyTopic)
		assert.Len(t, messages[emptyTopic], 1)
		assert.Equal(t, message, messages[emptyTopic][0])
	})
}
