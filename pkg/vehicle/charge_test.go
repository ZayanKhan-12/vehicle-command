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

func TestScheduleChargingDeliversPublishedAction(t *testing.T) {
	car, sender := newTestVehicle()
	sender.Listen(nil)
	sender.fixedResponse = &universal.RoutableMessage{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := car.ScheduleCharging(ctx, true, 2*time.Hour); err != nil {
		t.Fatalf("ScheduleCharging: %v", err)
	}
	if sender.lastMessage == nil {
		t.Fatal("dispatcher did not capture a request")
	}
	var action carserver.Action
	if err := proto.Unmarshal(sender.lastMessage.GetProtobufMessageAsBytes(), &action); err != nil {
		t.Fatalf("unmarshal Action: %v", err)
	}
	got := action.GetVehicleAction().GetScheduledChargingAction()
	if got == nil {
		t.Fatalf("request was not ScheduledChargingAction: %v", &action)
	}
	if !got.GetEnabled() {
		t.Error("enabled = false, want true")
	}
	if got.GetChargingTime() != 120 {
		t.Errorf("charging_time = %d, want 120 minutes", got.GetChargingTime())
	}
}

func TestScheduleChargingDoesNotReturnFirmwareWorkaroundError(t *testing.T) {
	car, sender := newTestVehicle()
	sender.Listen(nil)
	sender.fixedResponse = &universal.RoutableMessage{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := car.ScheduleCharging(ctx, true, time.Hour)
	if errors.Is(err, protocol.ErrScheduledChargingFirmware) {
		t.Fatal("ScheduleCharging must still send the published action; firmware sleep/wake is not a client-side refusal")
	}
	if err != nil {
		t.Fatalf("ScheduleCharging: %v", err)
	}
}
