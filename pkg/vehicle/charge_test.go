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

func TestChargeStopDeliversPublishedAction(t *testing.T) {
	car, sender := newTestVehicle()
	sender.Listen(nil)
	sender.fixedResponse = &universal.RoutableMessage{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := car.ChargeStop(ctx); err != nil {
		if errors.Is(err, protocol.ErrChargingWhileInfotainmentAsleep) {
			t.Fatal("ChargeStop must still send ChargingStartStopAction; Infotainment sleep is Tesla signed_command, not a client-side refusal")
		}
		t.Fatalf("ChargeStop: %v", err)
	}
	if sender.lastMessage == nil {
		t.Fatal("dispatcher did not capture a request")
	}
	var action carserver.Action
	if err := proto.Unmarshal(sender.lastMessage.GetProtobufMessageAsBytes(), &action); err != nil {
		t.Fatalf("unmarshal Action: %v", err)
	}
	stop := action.GetVehicleAction().GetChargingStartStopAction()
	if stop == nil || stop.GetStop() == nil {
		t.Fatalf("request was not ChargingStartStopAction stop: %v", &action)
	}
}

func TestSetChargingAmpsDeliversPublishedAction(t *testing.T) {
	car, sender := newTestVehicle()
	sender.Listen(nil)
	sender.fixedResponse = &universal.RoutableMessage{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := car.SetChargingAmps(ctx, 16); err != nil {
		if errors.Is(err, protocol.ErrChargingWhileInfotainmentAsleep) {
			t.Fatal("SetChargingAmps must still send SetChargingAmpsAction; Infotainment sleep is Tesla signed_command, not a client-side refusal")
		}
		t.Fatalf("SetChargingAmps: %v", err)
	}
	if sender.lastMessage == nil {
		t.Fatal("dispatcher did not capture a request")
	}
	var action carserver.Action
	if err := proto.Unmarshal(sender.lastMessage.GetProtobufMessageAsBytes(), &action); err != nil {
		t.Fatalf("unmarshal Action: %v", err)
	}
	got := action.GetVehicleAction().GetSetChargingAmpsAction()
	if got == nil {
		t.Fatalf("request was not SetChargingAmpsAction: %v", &action)
	}
	if got.GetChargingAmps() != 16 {
		t.Errorf("charging_amps = %d, want 16", got.GetChargingAmps())
	}
}
