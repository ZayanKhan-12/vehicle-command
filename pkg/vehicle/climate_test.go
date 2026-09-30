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

func climateActionFromLastMessage(t *testing.T, sender *testSender) *carserver.VehicleAction {
	t.Helper()
	if sender.lastMessage == nil {
		t.Fatal("dispatcher did not capture a request")
	}
	var action carserver.Action
	if err := proto.Unmarshal(sender.lastMessage.GetProtobufMessageAsBytes(), &action); err != nil {
		t.Fatalf("unmarshal Action: %v", err)
	}
	got := action.GetVehicleAction()
	if got == nil {
		t.Fatalf("request was not VehicleAction: %v", &action)
	}
	return got
}

func TestSetClimateKeeperModeDeliversPublishedAction(t *testing.T) {
	car, sender := newTestVehicle()
	sender.Listen(nil)
	sender.fixedResponse = &universal.RoutableMessage{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := car.SetClimateKeeperMode(ctx, ClimateKeeperModeDog, false); err != nil {
		if errors.Is(err, protocol.ErrClimateKeeperCPDFirmware) {
			t.Fatal("SetClimateKeeperMode must still send HvacClimateKeeperAction; cpd_enabled is firmware NominalError, not a client-side refusal")
		}
		t.Fatalf("SetClimateKeeperMode: %v", err)
	}
	got := climateActionFromLastMessage(t, sender).GetHvacClimateKeeperAction()
	if got == nil {
		t.Fatal("request was not HvacClimateKeeperAction")
	}
	if got.GetClimateKeeperAction() != ClimateKeeperModeDog {
		t.Errorf("mode = %v, want Dog", got.GetClimateKeeperAction())
	}
	if got.GetManualOverride() {
		t.Error("manual_override = true, want false")
	}
}

func TestSetClimateKeeperModeManualOverrideIsNotCPDBypass(t *testing.T) {
	car, sender := newTestVehicle()
	sender.Listen(nil)
	sender.fixedResponse = &universal.RoutableMessage{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := car.SetClimateKeeperMode(ctx, ClimateKeeperModeCamp, true); err != nil {
		if errors.Is(err, protocol.ErrClimateKeeperCPDFirmware) {
			t.Fatal("manual_override must still be sent as the published low-SOC bit; firmware may still refuse with cpd_enabled")
		}
		t.Fatalf("SetClimateKeeperMode: %v", err)
	}
	got := climateActionFromLastMessage(t, sender).GetHvacClimateKeeperAction()
	if got == nil {
		t.Fatal("request was not HvacClimateKeeperAction")
	}
	if got.GetClimateKeeperAction() != ClimateKeeperModeCamp {
		t.Errorf("mode = %v, want Camp", got.GetClimateKeeperAction())
	}
	if !got.GetManualOverride() {
		t.Error("manual_override = false, want true")
	}
}
