package runner

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
)

const (
	// DQLReadRaftMetadata is used to query the raft metadata
	DQLReadRaftMetadata = `
SELECT data FROM metadata WHERE namespace=?;
	`
)

const (
	// DMLSaveRaftMetadata write metadata
	DMLWriteRaftMetadata = `
INSERT INTO metadata (namespace, data) VALUES (?, ?);
	`
)
