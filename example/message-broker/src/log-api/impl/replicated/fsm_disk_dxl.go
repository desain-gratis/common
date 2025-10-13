package replicated

const (
	// DDLRaftMetadata used to store raft metadata such as last applied index
	DDLRaftMetadata = `
CREATE TABLE IF NOT EXISTS metadata (
	namespace String,
	data String,
)
ENGINE = ReplacingMergeTree
ORDER BY namespace;
`

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
	// DQLReadRaftMetadata is used to query the raft metadata
	DQLReadRaftMetadata = `
SELECT data FROM metadata WHERE namespace='default';
	`

	// DQLReadChatFromOffset read chat
	DQLReadChatFromOffset = `
SELECT namespace, event_id, server_timestamp, data FROM chat_log WHERE event_id < ? ORDER BY event_id DESC;
`
)

const (
	// DMLSaveRaftMetadata write metadata
	DMLWriteRaftMetadata = `
INSERT INTO metadata (namespace, data) VALUES ('default', ?);
	`

	// DMLWriteChat write chat data
	DMLWriteChat = `
INSERT INTO chat_log (namespace, event_id, server_timestamp, data);
`
)
