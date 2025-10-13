package replicated

const (
	chatTableKey ContextKey = "chat-table"
	chConnKey    ContextKey = "clickhouse-conn"
	metadataKey  ContextKey = "metadata"
)

type ContextKey string
