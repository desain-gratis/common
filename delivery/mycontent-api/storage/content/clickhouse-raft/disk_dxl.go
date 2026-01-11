package clickhouseraft

const (
	// DDLChatLog to create the chat table
	DDLChatLog string = `
CREATE TABLE IF NOT EXISTS chat_log (
	namespace String,
	event_id UInt64,
	server_timestamp DateTime,
	data String,
) ENGINE = ReplacingMergeTree 
ORDER BY (namespace, event_id);
`
)

const (

	// DQLReadAll read log
	DQLReadAll = `
SELECT namespace, event_id, server_timestamp, data FROM chat_log WHERE event_id < ? ORDER BY event_id ASC;
`

	// DQLReadAllFromDateTime read log
	DQLReadAllFromDateTime = `
SELECT namespace, event_id, server_timestamp, data FROM chat_log WHERE event_id < ? AND server_timestamp >= ? ORDER BY event_id ASC;
`
)

const (
	// DMLWriteMetadata write metadata
	DMLWriteMetadata = `
INSERT INTO metadata (namespace, data) VALUES (?, ?);
	`

	// DMLWriteChatBatch write chat data with batch
	DMLWriteChatBatch = `
INSERT INTO chat_log (namespace, event_id, server_timestamp, data);
`

	// DMLWriteChat write chat data
	DMLWriteChat = `
INSERT INTO chat_log (namespace, event_id, server_timestamp, data) VALUES (?, ?, ?, ?);
`
)
