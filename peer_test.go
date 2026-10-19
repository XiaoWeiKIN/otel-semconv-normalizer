// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"reflect"
	"testing"

	"github.com/XiaoWeiKIN/otel-semconv-normalizer/internal/semconvmeta"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	semconv1_16 "go.opentelemetry.io/otel/semconv/v1.16.0"
	semconv1_25 "go.opentelemetry.io/otel/semconv/v1.25.0"
	semconv1_34 "go.opentelemetry.io/otel/semconv/v1.34.0"
	semconv1_41 "go.opentelemetry.io/otel/semconv/v1.41.0"
)

func TestPeerSourceKeysMatchOfficialSemconv(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		// peer.service and rpc.service disappeared from semconv/v1.39.0.
		// Keep their newest retained project constants instead of literals.
		{name: "peer service", got: keyPeerService, want: string(semconv1_25.PeerServiceKey)},
		{name: "RPC service", got: keyRPCService, want: string(semconv1_34.RPCServiceKey)},
		{name: "server address", got: keyServerAddress, want: string(semconv1_41.ServerAddressKey)},
		{name: "legacy network peer name", got: keyNetPeerName, want: string(semconv1_25.NetPeerNameKey)},
		{name: "DB namespace", got: keyDBNamespace, want: string(semconv1_41.DBNamespaceKey)},
		{name: "legacy DB name", got: keyDBName, want: string(semconv1_25.DBNameKey)},
		{name: "messaging destination", got: keyMessagingDestinationName, want: string(semconv1_41.MessagingDestinationNameKey)},
		{name: "legacy messaging destination", got: keyMessagingDestination, want: string(semconv1_16.MessagingDestinationKey)},
		{name: "DynamoDB table names", got: keyAWSDynamoDBTableNames, want: string(semconv1_41.AWSDynamoDBTableNamesKey)},
		{name: "S3 bucket", got: keyAWSS3Bucket, want: string(semconv1_41.AWSS3BucketKey)},
		{name: "Kinesis stream", got: keyAWSKinesisStreamName, want: string(semconv1_41.AWSKinesisStreamNameKey)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("key = %q, want %q", test.got, test.want)
			}
		})
	}
}

func TestPeerFieldRegistryShape(t *testing.T) {
	tests := []struct {
		id          semconvmeta.FieldID
		name        string
		keys        [5]string
		keyCount    uint8
		valueType   semconvmeta.ValueType
		valuePolicy semconvmeta.ValuePolicy
	}{
		{id: semconvmeta.FieldPeerService, name: "peer.service", keys: [5]string{keyPeerService}, keyCount: 1, valueType: semconvmeta.ValueTypeString, valuePolicy: semconvmeta.ValueIdentity},
		{id: semconvmeta.FieldPeerHostname, name: "peer.hostname", keys: [5]string{keyServerAddress, keyNetPeerName}, keyCount: 2, valueType: semconvmeta.ValueTypeString, valuePolicy: semconvmeta.ValueIdentity},
		{id: semconvmeta.FieldPeerDBName, name: "peer.db.name", keys: [5]string{keyDBNamespace, keyDBName}, keyCount: 2, valueType: semconvmeta.ValueTypeString, valuePolicy: semconvmeta.ValueIdentity},
		{id: semconvmeta.FieldPeerMessagingDestination, name: "peer.messaging.destination", keys: [5]string{keyMessagingDestinationName, keyMessagingDestination}, keyCount: 2, valueType: semconvmeta.ValueTypeString, valuePolicy: semconvmeta.ValueIdentity},
		{id: semconvmeta.FieldPeerRPCService, name: "peer.rpc.service", keys: [5]string{keyRPCService}, keyCount: 1, valueType: semconvmeta.ValueTypeString, valuePolicy: semconvmeta.ValueIdentity},
		{id: semconvmeta.FieldPeerAWSDynamoDBTable, name: "peer.aws.dynamodb.table", keys: [5]string{keyAWSDynamoDBTableNames}, keyCount: 1, valueType: semconvmeta.ValueTypeStringSlice, valuePolicy: semconvmeta.ValueIdentity},
		{id: semconvmeta.FieldPeerAWSS3Bucket, name: "peer.aws.s3.bucket", keys: [5]string{keyAWSS3Bucket}, keyCount: 1, valueType: semconvmeta.ValueTypeString, valuePolicy: semconvmeta.ValueIdentity},
		{id: semconvmeta.FieldPeerAWSKinesisStream, name: "peer.aws.kinesis.stream", keys: [5]string{keyAWSKinesisStreamName}, keyCount: 1, valueType: semconvmeta.ValueTypeString, valuePolicy: semconvmeta.ValueIdentity},
		{id: semconvmeta.FieldPeerDBSystem, name: "peer.db.system", keys: [5]string{keyDBSystemName, keyDBSystem, keyDBType}, keyCount: 3, valueType: semconvmeta.ValueTypeString, valuePolicy: semconvmeta.ValueTechnology},
		{id: semconvmeta.FieldPeerMessagingSystem, name: "peer.messaging.system", keys: [5]string{keyMessagingSystem}, keyCount: 1, valueType: semconvmeta.ValueTypeString, valuePolicy: semconvmeta.ValueTechnology},
		{id: semconvmeta.FieldPeerRPCSystem, name: "peer.rpc.system", keys: [5]string{keyRPCSystemName, keyRPCSystem}, keyCount: 2, valueType: semconvmeta.ValueTypeString, valuePolicy: semconvmeta.ValueTechnology},
		{id: semconvmeta.FieldPeerKafkaBootstrapServers, name: "peer.kafka.bootstrap.servers", keys: [5]string{keyMessagingKafkaBootstrapServers}, keyCount: 1, valueType: semconvmeta.ValueTypeString, valuePolicy: semconvmeta.ValueIdentity},
		{id: semconvmeta.FieldPeerRocketMQBroker, name: "peer.rocketmq.broker", keys: [5]string{keyMessagingRocketMQBrokerAddress}, keyCount: 1, valueType: semconvmeta.ValueTypeString, valuePolicy: semconvmeta.ValueIdentity},
	}

	if got := int(semconvmeta.FieldCount - semconvmeta.CoreFieldCount); got != len(tests) {
		t.Fatalf("peer field count = %d, want %d", got, len(tests))
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := semconvmeta.Field(test.id)
			if string(got.Concept()) != test.name {
				t.Errorf("concept = %q, want %q", got.Concept(), test.name)
			}
			if got.SourceKeys() != test.keys {
				t.Errorf("sourceKeys = %q, want %q", got.SourceKeys(), test.keys)
			}
			if got.SourceKeyCount() != test.keyCount {
				t.Errorf("sourceKeyCount = %d, want %d", got.SourceKeyCount(), test.keyCount)
			}
			if got.ValueType() != test.valueType {
				t.Errorf("valueType = %d, want %d", got.ValueType(), test.valueType)
			}
			if got.ValuePolicy() != test.valuePolicy {
				t.Errorf("valuePolicy = %d, want %d", got.ValuePolicy(), test.valuePolicy)
			}
			if got.KindPolicy() != semconvmeta.KindPeer {
				t.Errorf("kindPolicy = %d, want peer-kind", got.KindPolicy())
			}
		})
	}
}

func TestPeerSpanKindGate(t *testing.T) {
	tests := []struct {
		kind ptrace.SpanKind
		want bool
	}{
		{kind: ptrace.SpanKindClient, want: true},
		{kind: ptrace.SpanKindProducer, want: true},
		{kind: ptrace.SpanKindConsumer, want: true},
		{kind: ptrace.SpanKindServer, want: false},
		{kind: ptrace.SpanKindInternal, want: false},
		{kind: ptrace.SpanKindUnspecified, want: false},
	}

	for _, test := range tests {
		t.Run(test.kind.String(), func(t *testing.T) {
			if got := isPeerSpanKind(test.kind); got != test.want {
				t.Fatalf("isPeerSpanKind(%s) = %v, want %v", test.kind, got, test.want)
			}
		})
	}
}

func TestGetPeerStringFields(t *testing.T) {
	t.Run("every concept remains an independent column", func(t *testing.T) {
		attrs := testAttrs(map[string]string{
			keyPeerService:              "checkout-service",
			keyServerAddress:            "db.internal",
			keyDBNamespace:              "orders",
			keyMessagingDestinationName: "orders.created",
			keyRPCService:               "orders.v1.OrderService",
			keyAWSS3Bucket:              "invoice-archive",
			keyAWSKinesisStreamName:     "order-events",
		})

		tests := []struct {
			name string
			get  func(pcommon.Map, ptrace.SpanKind) string
			want string
		}{
			{name: "service", get: GetPeerService, want: "checkout-service"},
			{name: "hostname", get: GetPeerHostname, want: "db.internal"},
			{name: "DB name", get: GetPeerDBName, want: "orders"},
			{name: "messaging destination", get: GetPeerMessagingDestination, want: "orders.created"},
			{name: "RPC service", get: GetPeerRPCService, want: "orders.v1.OrderService"},
			{name: "S3 bucket", get: GetPeerAWSS3Bucket, want: "invoice-archive"},
			{name: "Kinesis stream", get: GetPeerAWSKinesisStream, want: "order-events"},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if got := test.get(attrs, ptrace.SpanKindClient); got != test.want {
					t.Fatalf("getter() = %q, want %q", got, test.want)
				}
			})
		}
	})

	t.Run("current source wins and empty falls back", func(t *testing.T) {
		tests := []struct {
			name  string
			attrs map[string]string
			get   func(pcommon.Map, ptrace.SpanKind) string
			want  string
		}{
			{
				name:  "hostname current wins",
				attrs: map[string]string{keyServerAddress: "current.example", keyNetPeerName: "legacy.example"},
				get:   GetPeerHostname,
				want:  "current.example",
			},
			{
				name:  "hostname current empty falls back",
				attrs: map[string]string{keyServerAddress: "", keyNetPeerName: "legacy.example"},
				get:   GetPeerHostname,
				want:  "legacy.example",
			},
			{
				name:  "DB current wins",
				attrs: map[string]string{keyDBNamespace: "current-db", keyDBName: "legacy-db"},
				get:   GetPeerDBName,
				want:  "current-db",
			},
			{
				name:  "DB current empty falls back",
				attrs: map[string]string{keyDBNamespace: "", keyDBName: "legacy-db"},
				get:   GetPeerDBName,
				want:  "legacy-db",
			},
			{
				name:  "messaging destination current wins",
				attrs: map[string]string{keyMessagingDestinationName: "current-queue", keyMessagingDestination: "legacy-queue"},
				get:   GetPeerMessagingDestination,
				want:  "current-queue",
			},
			{
				name:  "messaging destination current empty falls back",
				attrs: map[string]string{keyMessagingDestinationName: "", keyMessagingDestination: "legacy-queue"},
				get:   GetPeerMessagingDestination,
				want:  "legacy-queue",
			},
			{
				name:  "missing returns empty",
				attrs: map[string]string{},
				get:   GetPeerService,
				want:  "",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if got := test.get(testAttrs(test.attrs), ptrace.SpanKindClient); got != test.want {
					t.Fatalf("getter() = %q, want %q", got, test.want)
				}
			})
		}
	})
}

func TestGetPeerSystemFields(t *testing.T) {
	tests := []struct {
		name  string
		attrs map[string]string
		get   func(pcommon.Map, ptrace.SpanKind) string
		want  string
	}{
		{name: "db system canonicalized", attrs: map[string]string{keyDBSystem: "mssql"}, get: GetPeerDBSystem, want: "microsoft.sql_server"},
		{name: "db system new name passes through", attrs: map[string]string{keyDBSystemName: "postgresql"}, get: GetPeerDBSystem, want: "postgresql"},
		{name: "messaging system", attrs: map[string]string{keyMessagingSystem: "kafka"}, get: GetPeerMessagingSystem, want: "kafka"},
		{name: "rpc system", attrs: map[string]string{keyRPCSystem: "grpc"}, get: GetPeerRPCSystem, want: "grpc"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.get(testAttrs(test.attrs), ptrace.SpanKindClient); got != test.want {
				t.Errorf("getter = %q, want %q", got, test.want)
			}
			if got := test.get(testAttrs(test.attrs), ptrace.SpanKindServer); got != "" {
				t.Errorf("server getter = %q, want empty (kind gate)", got)
			}
		})
	}
}

func TestGetPeerKafkaBootstrapServers(t *testing.T) {
	attrs := testAttrs(map[string]string{keyMessagingKafkaBootstrapServers: "b1:9092,b2:9092"})
	if got := GetPeerKafkaBootstrapServers(attrs, ptrace.SpanKindProducer); got != "b1:9092,b2:9092" {
		t.Errorf("GetPeerKafkaBootstrapServers() = %q, want %q", got, "b1:9092,b2:9092")
	}
	if got := GetPeerKafkaBootstrapServers(attrs, ptrace.SpanKindServer); got != "" {
		t.Errorf("server getter = %q, want empty (kind gate)", got)
	}
}

func TestGetPeerRocketMQBroker(t *testing.T) {
	attrs := testAttrs(map[string]string{keyMessagingRocketMQBrokerAddress: "broker-a:10911"})
	if got := GetPeerRocketMQBroker(attrs, ptrace.SpanKindConsumer); got != "broker-a:10911" {
		t.Errorf("GetPeerRocketMQBroker() = %q, want %q", got, "broker-a:10911")
	}
	if got := GetPeerRocketMQBroker(attrs, ptrace.SpanKindServer); got != "" {
		t.Errorf("server getter = %q, want empty (kind gate)", got)
	}
}

func TestGetPeerAWSDynamoDBTable(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		kind   ptrace.SpanKind
		want   string
	}{
		{name: "first table", values: []string{"orders", "archive"}, kind: ptrace.SpanKindClient, want: "orders"},
		{name: "first non-empty table", values: []string{"", "orders", "archive"}, kind: ptrace.SpanKindClient, want: "orders"},
		{name: "empty list", values: []string{}, kind: ptrace.SpanKindClient, want: ""},
		{name: "server is excluded", values: []string{"orders"}, kind: ptrace.SpanKindServer, want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			attrs := pcommon.NewMap()
			tables := attrs.PutEmptySlice(keyAWSDynamoDBTableNames)
			tables.EnsureCapacity(len(test.values))
			for _, value := range test.values {
				tables.AppendEmpty().SetStr(value)
			}

			if got := GetPeerAWSDynamoDBTable(attrs, test.kind); got != test.want {
				t.Fatalf("GetPeerAWSDynamoDBTable() = %q, want %q", got, test.want)
			}
		})
	}

	t.Run("wrong attribute type is ignored", func(t *testing.T) {
		attrs := testAttrs(map[string]string{keyAWSDynamoDBTableNames: "orders"})
		if got := GetPeerAWSDynamoDBTable(attrs, ptrace.SpanKindClient); got != "" {
			t.Fatalf("GetPeerAWSDynamoDBTable() = %q, want empty", got)
		}
	})
}

func TestPeerGettersRespectKindGateAndDoNotMutate(t *testing.T) {
	attrs := testAttrs(map[string]string{
		keyPeerService:              "checkout-service",
		keyServerAddress:            "api.internal",
		keyNetPeerName:              "legacy.internal",
		keyDBNamespace:              "orders",
		keyDBName:                   "legacy-orders",
		keyMessagingDestinationName: "orders.created",
		keyRPCService:               "orders.v1.OrderService",
		keyAWSS3Bucket:              "invoice-archive",
		keyAWSKinesisStreamName:     "order-events",
		keyDDOperationName:          "custom.operation",
		keyServiceName:              "custom-service-override",
	})
	attrs.PutEmptySlice(keyAWSDynamoDBTableNames).AppendEmpty().SetStr("orders")

	getters := []struct {
		name string
		get  func(pcommon.Map, ptrace.SpanKind) string
		want string
	}{
		{name: "service", get: GetPeerService, want: "checkout-service"},
		{name: "hostname", get: GetPeerHostname, want: "api.internal"},
		{name: "DB name", get: GetPeerDBName, want: "orders"},
		{name: "messaging destination", get: GetPeerMessagingDestination, want: "orders.created"},
		{name: "RPC service", get: GetPeerRPCService, want: "orders.v1.OrderService"},
		{name: "DynamoDB table", get: GetPeerAWSDynamoDBTable, want: "orders"},
		{name: "S3 bucket", get: GetPeerAWSS3Bucket, want: "invoice-archive"},
		{name: "Kinesis stream", get: GetPeerAWSKinesisStream, want: "order-events"},
	}

	before := attrs.AsRaw()
	for _, kind := range []ptrace.SpanKind{
		ptrace.SpanKindClient,
		ptrace.SpanKindProducer,
		ptrace.SpanKindConsumer,
	} {
		t.Run("allowed "+kind.String(), func(t *testing.T) {
			for _, getter := range getters {
				if got := getter.get(attrs, kind); got != getter.want {
					t.Errorf("%s getter = %q, want %q", getter.name, got, getter.want)
				}
			}
		})
	}

	for _, kind := range []ptrace.SpanKind{
		ptrace.SpanKindServer,
		ptrace.SpanKindInternal,
		ptrace.SpanKindUnspecified,
	} {
		t.Run("excluded "+kind.String(), func(t *testing.T) {
			for _, getter := range getters {
				if got := getter.get(attrs, kind); got != "" {
					t.Errorf("%s getter = %q, want empty", getter.name, got)
				}
			}
		})
	}

	if after := attrs.AsRaw(); !reflect.DeepEqual(after, before) {
		t.Fatalf("peer getters mutated attributes: before=%v, after=%v", before, after)
	}
}

func TestPeerGettersIgnoreExcludedDataDogSources(t *testing.T) {
	excluded := []string{
		"peer.hostname",
		"hostname",
		"db.hostname",
		"network.destination.name",
		"grpc.host",
		"http.host",
		"http.server_name",
		"db.instance",
		"mongodb.db",
		"cassandra.keyspace",
		"db.cassandra.contact.points",
		"db.couchbase.seed.nodes",
		"tablename",
		"bucketname",
		"streamname",
		"queuename",
		"topicname",
		"aws.queue.name",
		"_dd.base_service",
	}
	attrs := pcommon.NewMap()
	for _, key := range excluded {
		attrs.PutStr(key, "must-not-be-consumed")
	}

	getters := []struct {
		name string
		get  func(pcommon.Map, ptrace.SpanKind) string
	}{
		{name: "service", get: GetPeerService},
		{name: "hostname", get: GetPeerHostname},
		{name: "DB name", get: GetPeerDBName},
		{name: "messaging destination", get: GetPeerMessagingDestination},
		{name: "RPC service", get: GetPeerRPCService},
		{name: "DynamoDB table", get: GetPeerAWSDynamoDBTable},
		{name: "S3 bucket", get: GetPeerAWSS3Bucket},
		{name: "Kinesis stream", get: GetPeerAWSKinesisStream},
	}
	for _, getter := range getters {
		t.Run(getter.name, func(t *testing.T) {
			if got := getter.get(attrs, ptrace.SpanKindClient); got != "" {
				t.Fatalf("getter consumed an excluded source: got %q", got)
			}
		})
	}
}
