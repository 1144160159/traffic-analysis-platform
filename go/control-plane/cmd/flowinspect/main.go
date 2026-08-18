package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"strconv"
	"time"

	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/scram"
	"google.golang.org/protobuf/proto"
)

func main() {
	mechanism, _ := scram.Mechanism(scram.SHA512, "traffic-broker", os.Getenv("KAFKA_PW"))
	dialer := &kafka.Dialer{Timeout: 10 * time.Second, SASLMechanism: mechanism, TLS: &tls.Config{InsecureSkipVerify: true}}
	part, _ := strconv.Atoi(os.Args[1])
	off, _ := strconv.ParseInt(os.Args[2], 10, 64)
	r := kafka.NewReader(kafka.ReaderConfig{Brokers: []string{os.Args[3]}, Topic: "flow.events.v1", Partition: part, Dialer: dialer, MinBytes: 1, MaxBytes: 10 << 20, MaxWait: 300 * time.Millisecond})
	defer r.Close()
	_ = r.SetOffset(off)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for i := 0; i < 6; i++ {
		m, err := r.FetchMessage(ctx)
		if err != nil {
			fmt.Println("fetch:", err)
			return
		}
		var f pb.FlowEvent
		if err := proto.Unmarshal(m.Value, &f); err != nil {
			fmt.Println("unmarshal:", err)
			return
		}
		fo := f.GetFeatureObservation()
		sec := ""
		missing := 0
		if fo != nil {
			sec = fo.GetTransportSecurity().String()
			missing = len(fo.GetMissingFields())
		}
		fmt.Printf("off=%d fo=%v transport_security=%s tls=%s ja3=%s missing=%d payload_observed=%d\n",
			m.Offset, fo != nil, sec, fo.GetTlsVersion(), fo.GetJa3(), missing, fo.GetPayloadObservedBytes())
	}
}
