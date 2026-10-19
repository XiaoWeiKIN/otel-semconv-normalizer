// SPDX-License-Identifier: Apache-2.0

// Package semconvmeta owns the build-neutral canonical key order and value aliases
// shared by root getters and the SemConv weave compiler.
package semconvmeta

type FieldID uint8

const (
	FieldHTTPRequestMethod FieldID = iota
	FieldHTTPRequestMethodOriginal
	FieldHTTPResponseStatusCode
	FieldURLFull
	FieldURLScheme
	FieldURLTemplate
	FieldHTTPRoute
	FieldUserAgentOriginal
	FieldNetworkProtocolName
	FieldNetworkTransport
	FieldDBOperationName
	FieldDBCollectionName
	FieldDBNamespace
	FieldDBQuerySummary
	FieldDBQueryText
	FieldPeerService
	FieldPeerHostname
	FieldPeerDBName
	FieldPeerMessagingDestination
	FieldPeerRPCService
	FieldPeerAWSDynamoDBTable
	FieldPeerAWSS3Bucket
	FieldPeerAWSKinesisStream
	FieldPeerDBSystem
	FieldPeerMessagingSystem
	FieldPeerRPCSystem
	FieldPeerKafkaBootstrapServers
	FieldPeerRocketMQBroker
	FieldCount
)

const CoreFieldCount FieldID = FieldPeerService

type ValueType uint8

const (
	ValueTypeString ValueType = iota + 1
	ValueTypeInt64
	ValueTypeStringSlice
)

type ValuePolicy uint8

const (
	ValueIdentity ValuePolicy = iota
	ValueHTTPMethod
	ValueHTTPMethodOriginal
	ValueNetworkTransport
	ValueTechnology
)

type KindPolicy uint8

const (
	KindAny KindPolicy = iota
	KindPeer
)

type fieldDescriptor struct {
	Concept        string
	SourceKeys     [5]string
	SourceKeyCount uint8
	ValueType      ValueType
	ValuePolicy    ValuePolicy
	KindPolicy     KindPolicy
}

// FieldDescriptor is an immutable view over one semantic field descriptor.
// Its backing pointer is deliberately private so runtime callers can read the
// static table without copying it or gaining mutation access.
type FieldDescriptor struct {
	field *fieldDescriptor
}

var fields = [FieldCount]fieldDescriptor{
	FieldHTTPRequestMethod: {
		Concept:        "http.request.method",
		SourceKeys:     [5]string{"http.request.method", "http.method"},
		SourceKeyCount: 2,
		ValueType:      ValueType(1),
		ValuePolicy:    ValuePolicy(1),
		KindPolicy:     KindPolicy(0),
	},
	FieldHTTPRequestMethodOriginal: {
		Concept:        "http.request.method_original",
		SourceKeys:     [5]string{"http.request.method_original"},
		SourceKeyCount: 1,
		ValueType:      ValueType(1),
		ValuePolicy:    ValuePolicy(2),
		KindPolicy:     KindPolicy(0),
	},
	FieldHTTPResponseStatusCode: {
		Concept:        "http.response.status_code",
		SourceKeys:     [5]string{"http.response.status_code", "http.status_code"},
		SourceKeyCount: 2,
		ValueType:      ValueType(2),
		ValuePolicy:    ValuePolicy(0),
		KindPolicy:     KindPolicy(0),
	},
	FieldURLFull: {
		Concept:        "url.full",
		SourceKeys:     [5]string{"url.full", "http.url"},
		SourceKeyCount: 2,
		ValueType:      ValueType(1),
		ValuePolicy:    ValuePolicy(0),
		KindPolicy:     KindPolicy(0),
	},
	FieldURLScheme: {
		Concept:        "url.scheme",
		SourceKeys:     [5]string{"url.scheme", "http.scheme"},
		SourceKeyCount: 2,
		ValueType:      ValueType(1),
		ValuePolicy:    ValuePolicy(0),
		KindPolicy:     KindPolicy(0),
	},
	FieldURLTemplate: {
		Concept:        "url.template",
		SourceKeys:     [5]string{"url.template"},
		SourceKeyCount: 1,
		ValueType:      ValueType(1),
		ValuePolicy:    ValuePolicy(0),
		KindPolicy:     KindPolicy(0),
	},
	FieldHTTPRoute: {
		Concept:        "http.route",
		SourceKeys:     [5]string{"http.route"},
		SourceKeyCount: 1,
		ValueType:      ValueType(1),
		ValuePolicy:    ValuePolicy(0),
		KindPolicy:     KindPolicy(0),
	},
	FieldUserAgentOriginal: {
		Concept:        "user_agent.original",
		SourceKeys:     [5]string{"user_agent.original", "http.user_agent"},
		SourceKeyCount: 2,
		ValueType:      ValueType(1),
		ValuePolicy:    ValuePolicy(0),
		KindPolicy:     KindPolicy(0),
	},
	FieldNetworkProtocolName: {
		Concept:        "network.protocol.name",
		SourceKeys:     [5]string{"network.protocol.name", "net.protocol.name"},
		SourceKeyCount: 2,
		ValueType:      ValueType(1),
		ValuePolicy:    ValuePolicy(0),
		KindPolicy:     KindPolicy(0),
	},
	FieldNetworkTransport: {
		Concept:        "network.transport",
		SourceKeys:     [5]string{"network.transport", "net.transport"},
		SourceKeyCount: 2,
		ValueType:      ValueType(1),
		ValuePolicy:    ValuePolicy(3),
		KindPolicy:     KindPolicy(0),
	},
	FieldDBOperationName: {
		Concept:        "db.operation.name",
		SourceKeys:     [5]string{"db.operation.name", "db.operation"},
		SourceKeyCount: 2,
		ValueType:      ValueType(1),
		ValuePolicy:    ValuePolicy(0),
		KindPolicy:     KindPolicy(0),
	},
	FieldDBCollectionName: {
		Concept:        "db.collection.name",
		SourceKeys:     [5]string{"db.collection.name", "db.sql.table", "db.cassandra.table", "db.mongodb.collection", "db.cosmosdb.container"},
		SourceKeyCount: 5,
		ValueType:      ValueType(1),
		ValuePolicy:    ValuePolicy(0),
		KindPolicy:     KindPolicy(0),
	},
	FieldDBNamespace: {
		Concept:        "db.namespace",
		SourceKeys:     [5]string{"db.namespace", "db.name", "db.redis.database_index", "db.mssql.instance_name"},
		SourceKeyCount: 4,
		ValueType:      ValueType(1),
		ValuePolicy:    ValuePolicy(0),
		KindPolicy:     KindPolicy(0),
	},
	FieldDBQuerySummary: {
		Concept:        "db.query.summary",
		SourceKeys:     [5]string{"db.query.summary"},
		SourceKeyCount: 1,
		ValueType:      ValueType(1),
		ValuePolicy:    ValuePolicy(0),
		KindPolicy:     KindPolicy(0),
	},
	FieldDBQueryText: {
		Concept:        "db.query.text",
		SourceKeys:     [5]string{"db.query.text", "db.statement"},
		SourceKeyCount: 2,
		ValueType:      ValueType(1),
		ValuePolicy:    ValuePolicy(0),
		KindPolicy:     KindPolicy(0),
	},
	FieldPeerService: {
		Concept:        "peer.service",
		SourceKeys:     [5]string{"peer.service"},
		SourceKeyCount: 1,
		ValueType:      ValueType(1),
		ValuePolicy:    ValuePolicy(0),
		KindPolicy:     KindPolicy(1),
	},
	FieldPeerHostname: {
		Concept:        "peer.hostname",
		SourceKeys:     [5]string{"server.address", "net.peer.name"},
		SourceKeyCount: 2,
		ValueType:      ValueType(1),
		ValuePolicy:    ValuePolicy(0),
		KindPolicy:     KindPolicy(1),
	},
	FieldPeerDBName: {
		Concept:        "peer.db.name",
		SourceKeys:     [5]string{"db.namespace", "db.name"},
		SourceKeyCount: 2,
		ValueType:      ValueType(1),
		ValuePolicy:    ValuePolicy(0),
		KindPolicy:     KindPolicy(1),
	},
	FieldPeerMessagingDestination: {
		Concept:        "peer.messaging.destination",
		SourceKeys:     [5]string{"messaging.destination.name", "messaging.destination"},
		SourceKeyCount: 2,
		ValueType:      ValueType(1),
		ValuePolicy:    ValuePolicy(0),
		KindPolicy:     KindPolicy(1),
	},
	FieldPeerRPCService: {
		Concept:        "peer.rpc.service",
		SourceKeys:     [5]string{"rpc.service"},
		SourceKeyCount: 1,
		ValueType:      ValueType(1),
		ValuePolicy:    ValuePolicy(0),
		KindPolicy:     KindPolicy(1),
	},
	FieldPeerAWSDynamoDBTable: {
		Concept:        "peer.aws.dynamodb.table",
		SourceKeys:     [5]string{"aws.dynamodb.table_names"},
		SourceKeyCount: 1,
		ValueType:      ValueType(3),
		ValuePolicy:    ValuePolicy(0),
		KindPolicy:     KindPolicy(1),
	},
	FieldPeerAWSS3Bucket: {
		Concept:        "peer.aws.s3.bucket",
		SourceKeys:     [5]string{"aws.s3.bucket"},
		SourceKeyCount: 1,
		ValueType:      ValueType(1),
		ValuePolicy:    ValuePolicy(0),
		KindPolicy:     KindPolicy(1),
	},
	FieldPeerAWSKinesisStream: {
		Concept:        "peer.aws.kinesis.stream",
		SourceKeys:     [5]string{"aws.kinesis.stream_name"},
		SourceKeyCount: 1,
		ValueType:      ValueType(1),
		ValuePolicy:    ValuePolicy(0),
		KindPolicy:     KindPolicy(1),
	},
	FieldPeerDBSystem: {
		Concept:        "peer.db.system",
		SourceKeys:     [5]string{"db.system.name", "db.system", "db.type"},
		SourceKeyCount: 3,
		ValueType:      ValueType(1),
		ValuePolicy:    ValuePolicy(4),
		KindPolicy:     KindPolicy(1),
	},
	FieldPeerMessagingSystem: {
		Concept:        "peer.messaging.system",
		SourceKeys:     [5]string{"messaging.system"},
		SourceKeyCount: 1,
		ValueType:      ValueType(1),
		ValuePolicy:    ValuePolicy(4),
		KindPolicy:     KindPolicy(1),
	},
	FieldPeerRPCSystem: {
		Concept:        "peer.rpc.system",
		SourceKeys:     [5]string{"rpc.system.name", "rpc.system"},
		SourceKeyCount: 2,
		ValueType:      ValueType(1),
		ValuePolicy:    ValuePolicy(4),
		KindPolicy:     KindPolicy(1),
	},
	FieldPeerKafkaBootstrapServers: {
		Concept:        "peer.kafka.bootstrap.servers",
		SourceKeys:     [5]string{"messaging.kafka.bootstrap.servers"},
		SourceKeyCount: 1,
		ValueType:      ValueType(1),
		ValuePolicy:    ValuePolicy(0),
		KindPolicy:     KindPolicy(1),
	},
	FieldPeerRocketMQBroker: {
		Concept:        "peer.rocketmq.broker",
		SourceKeys:     [5]string{"messaging.rocketmq.broker_address"},
		SourceKeyCount: 1,
		ValueType:      ValueType(1),
		ValuePolicy:    ValuePolicy(0),
		KindPolicy:     KindPolicy(1),
	},
}

// Field returns an immutable, allocation-free descriptor view for id. Invalid
// IDs return an empty view.
func Field(id FieldID) FieldDescriptor {
	if id >= FieldCount {
		return FieldDescriptor{}
	}
	return FieldDescriptor{field: &fields[id]}
}

// Concept returns the canonical concept name.
func (f FieldDescriptor) Concept() string {
	if f.field == nil {
		return ""
	}
	return f.field.Concept
}

// SourceKeys returns a copy of the ordered source-key array. It is intended for
// tests and build-time inspection; runtime code should use SourceKey.
func (f FieldDescriptor) SourceKeys() [5]string {
	if f.field == nil {
		return [5]string{}
	}
	return f.field.SourceKeys
}

// SourceKeyCount returns the number of ordered source keys.
func (f FieldDescriptor) SourceKeyCount() uint8 {
	if f.field == nil {
		return 0
	}
	return f.field.SourceKeyCount
}

// SourceKey returns one ordered source key without exposing the backing table.
func (f FieldDescriptor) SourceKey(index uint8) string {
	if f.field == nil || index >= f.field.SourceKeyCount {
		return ""
	}
	return f.field.SourceKeys[index]
}

// ValueType returns the field's value type.
func (f FieldDescriptor) ValueType() ValueType {
	if f.field == nil {
		return 0
	}
	return f.field.ValueType
}

// ValuePolicy returns the field's normalization policy.
func (f FieldDescriptor) ValuePolicy() ValuePolicy {
	if f.field == nil {
		return 0
	}
	return f.field.ValuePolicy
}

// KindPolicy returns the field's span-kind policy.
func (f FieldDescriptor) KindPolicy() KindPolicy {
	if f.field == nil {
		return 0
	}
	return f.field.KindPolicy
}

type TechnologyAlias struct {
	Domain            string `json:"domain"`
	Source            string `json:"source"`
	Target            string `json:"target"`
	ProjectionAllowed bool   `json:"projection_allowed"`
	Owner             string `json:"owner"`
	Rationale         string `json:"rationale"`
}

var technologyAliases = [...]TechnologyAlias{
	{Domain: "db", Source: "adabas", Target: "softwareag.adabas", ProjectionAllowed: true, Owner: "otel-semconv-normalizer/semconv-maintainers", Rationale: "Pinned DB/RPC migration Guide and canonical v1.41.0 value contract."},
	{Domain: "rpc", Source: "apache_dubbo", Target: "dubbo", ProjectionAllowed: true, Owner: "otel-semconv-normalizer/semconv-maintainers", Rationale: "Pinned DB/RPC migration Guide and canonical v1.41.0 value contract."},
	{Domain: "db", Source: "cache", Target: "intersystems.cache", ProjectionAllowed: false, Owner: "otel-semconv-normalizer/semconv-maintainers", Rationale: "Pinned DB/RPC migration Guide and canonical v1.41.0 value contract."},
	{Domain: "rpc", Source: "connect_rpc", Target: "connectrpc", ProjectionAllowed: true, Owner: "otel-semconv-normalizer/semconv-maintainers", Rationale: "Pinned DB/RPC migration Guide and canonical v1.41.0 value contract."},
	{Domain: "db", Source: "cosmosdb", Target: "azure.cosmosdb", ProjectionAllowed: true, Owner: "otel-semconv-normalizer/semconv-maintainers", Rationale: "Pinned DB/RPC migration Guide and canonical v1.41.0 value contract."},
	{Domain: "db", Source: "db2", Target: "ibm.db2", ProjectionAllowed: true, Owner: "otel-semconv-normalizer/semconv-maintainers", Rationale: "Pinned DB/RPC migration Guide and canonical v1.41.0 value contract."},
	{Domain: "db", Source: "dynamodb", Target: "aws.dynamodb", ProjectionAllowed: true, Owner: "otel-semconv-normalizer/semconv-maintainers", Rationale: "Pinned DB/RPC migration Guide and canonical v1.41.0 value contract."},
	{Domain: "db", Source: "firebird", Target: "firebirdsql", ProjectionAllowed: true, Owner: "otel-semconv-normalizer/semconv-maintainers", Rationale: "Pinned DB/RPC migration Guide and canonical v1.41.0 value contract."},
	{Domain: "db", Source: "h2", Target: "h2database", ProjectionAllowed: true, Owner: "otel-semconv-normalizer/semconv-maintainers", Rationale: "Pinned DB/RPC migration Guide and canonical v1.41.0 value contract."},
	{Domain: "db", Source: "hanadb", Target: "sap.hana", ProjectionAllowed: true, Owner: "otel-semconv-normalizer/semconv-maintainers", Rationale: "Pinned DB/RPC migration Guide and canonical v1.41.0 value contract."},
	{Domain: "db", Source: "informix", Target: "ibm.informix", ProjectionAllowed: true, Owner: "otel-semconv-normalizer/semconv-maintainers", Rationale: "Pinned DB/RPC migration Guide and canonical v1.41.0 value contract."},
	{Domain: "db", Source: "ingres", Target: "actian.ingres", ProjectionAllowed: true, Owner: "otel-semconv-normalizer/semconv-maintainers", Rationale: "Pinned DB/RPC migration Guide and canonical v1.41.0 value contract."},
	{Domain: "db", Source: "intersystems_cache", Target: "intersystems.cache", ProjectionAllowed: true, Owner: "otel-semconv-normalizer/semconv-maintainers", Rationale: "Pinned DB/RPC migration Guide and canonical v1.41.0 value contract."},
	{Domain: "db", Source: "maxdb", Target: "sap.maxdb", ProjectionAllowed: true, Owner: "otel-semconv-normalizer/semconv-maintainers", Rationale: "Pinned DB/RPC migration Guide and canonical v1.41.0 value contract."},
	{Domain: "db", Source: "mssql", Target: "microsoft.sql_server", ProjectionAllowed: true, Owner: "otel-semconv-normalizer/semconv-maintainers", Rationale: "Pinned DB/RPC migration Guide and canonical v1.41.0 value contract."},
	{Domain: "db", Source: "netezza", Target: "ibm.netezza", ProjectionAllowed: true, Owner: "otel-semconv-normalizer/semconv-maintainers", Rationale: "Pinned DB/RPC migration Guide and canonical v1.41.0 value contract."},
	{Domain: "db", Source: "oracle", Target: "oracle.db", ProjectionAllowed: true, Owner: "otel-semconv-normalizer/semconv-maintainers", Rationale: "Pinned DB/RPC migration Guide and canonical v1.41.0 value contract."},
	{Domain: "db", Source: "redshift", Target: "aws.redshift", ProjectionAllowed: true, Owner: "otel-semconv-normalizer/semconv-maintainers", Rationale: "Pinned DB/RPC migration Guide and canonical v1.41.0 value contract."},
	{Domain: "db", Source: "spanner", Target: "gcp.spanner", ProjectionAllowed: true, Owner: "otel-semconv-normalizer/semconv-maintainers", Rationale: "Pinned DB/RPC migration Guide and canonical v1.41.0 value contract."},
}

// TechnologyAliases returns a copy for build-time consumers.
func TechnologyAliases() []TechnologyAlias {
	result := make([]TechnologyAlias, len(technologyAliases))
	copy(result, technologyAliases[:])
	return result
}

// CanonicalizeTechnology performs a zero-allocation binary search over the single alias source.
func CanonicalizeTechnology(value string) string {
	low, high := 0, len(technologyAliases)
	for low < high {
		mid := int(uint(low+high) >> 1)
		item := technologyAliases[mid]
		if value < item.Source {
			high = mid
		} else if value > item.Source {
			low = mid + 1
		} else {
			return item.Target
		}
	}
	return value
}
