package vehicle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/teslamotors/vehicle-command/pkg/protocol"
	carserver "github.com/teslamotors/vehicle-command/pkg/protocol/protobuf/carserver"
	universal "github.com/teslamotors/vehicle-command/pkg/protocol/protobuf/universalmessage"
	"google.golang.org/protobuf/proto"
)

func TestOpenChargePortDeliversPublishedAction(t *testing.T) {
	car, sender := newTestVehicle()
	sender.Listen(nil)
	sender.fixedResponse = &universal.RoutableMessage{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := car.OpenChargePort(ctx); err != nil {
		if errors.Is(err, protocol.ErrChargingManagerChargePortFirmware) {
			t.Fatal("OpenChargePort must still send ChargePortDoorOpen; Charging Manager ACL is firmware")
		}
		t.Fatalf("OpenChargePort: %v", err)
	}
	if sender.lastMessage == nil {
		t.Fatal("dispatcher did not capture a request")
	}
	var action carserver.Action
	if err := proto.Unmarshal(sender.lastMessage.GetProtobufMessageAsBytes(), &action); err != nil {
		t.Fatalf("unmarshal Action: %v", err)
	}
	if action.GetVehicleAction().GetChargePortDoorOpen() == nil {
		t.Fatalf("request was not ChargePortDoorOpen: %v", &action)
	}
}

func TestCloseChargePortDeliversPublishedAction(t *testing.T) {
	car, sender := newTestVehicle()
	sender.Listen(nil)
	sender.fixedResponse = &universal.RoutableMessage{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := car.CloseChargePort(ctx); err != nil {
		if errors.Is(err, protocol.ErrChargingManagerChargePortFirmware) {
			t.Fatal("CloseChargePort must still send ChargePortDoorClose")
		}
		t.Fatalf("CloseChargePort: %v", err)
	}
	var action carserver.Action
	if err := proto.Unmarshal(sender.lastMessage.GetProtobufMessageAsBytes(), &action); err != nil {
		t.Fatalf("unmarshal Action: %v", err)
	}
	if action.GetVehicleAction().GetChargePortDoorClose() == nil {
		t.Fatalf("request was not ChargePortDoorClose: %v", &action)
	}
}
