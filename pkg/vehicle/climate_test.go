package vehicle

import (
	"context"
	"testing"
	"time"

	carserver "github.com/teslamotors/vehicle-command/pkg/protocol/protobuf/carserver"
	universal "github.com/teslamotors/vehicle-command/pkg/protocol/protobuf/universalmessage"
	"google.golang.org/protobuf/proto"
)

func climateAction(t *testing.T, sender *testSender) *carserver.HvacAutoAction {
	t.Helper()
	if sender.lastMessage == nil {
		t.Fatal("dispatcher did not capture a request")
	}
	var action carserver.Action
	if err := proto.Unmarshal(sender.lastMessage.GetProtobufMessageAsBytes(), &action); err != nil {
		t.Fatalf("unmarshal Action: %v", err)
	}
	hvac := action.GetVehicleAction().GetHvacAutoAction()
	if hvac == nil {
		t.Fatalf("request was not HvacAutoAction: %v", &action)
	}
	return hvac
}

func TestClimateOnDoesNotSetManualOverride(t *testing.T) {
	car, sender := newTestVehicle()
	sender.Listen(nil)
	sender.fixedResponse = &universal.RoutableMessage{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := car.ClimateOn(ctx); err != nil {
		t.Fatalf("ClimateOn: %v", err)
	}
	hvac := climateAction(t, sender)
	if !hvac.GetPowerOn() {
		t.Error("PowerOn = false, want true")
	}
	if hvac.GetManualOverride() {
		t.Error("ClimateOn must leave manual_override unset/false so existing callers keep the historical request")
	}
}

func TestSetClimateManualOverride(t *testing.T) {
	car, sender := newTestVehicle()
	sender.Listen(nil)
	sender.fixedResponse = &universal.RoutableMessage{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := car.SetClimate(ctx, true, true); err != nil {
		t.Fatalf("SetClimate: %v", err)
	}
	hvac := climateAction(t, sender)
	if !hvac.GetPowerOn() || !hvac.GetManualOverride() {
		t.Errorf("HvacAutoAction = power_on=%v override=%v, want both true", hvac.GetPowerOn(), hvac.GetManualOverride())
	}
}

func TestClimateOffClearsOverride(t *testing.T) {
	car, sender := newTestVehicle()
	sender.Listen(nil)
	sender.fixedResponse = &universal.RoutableMessage{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := car.ClimateOff(ctx); err != nil {
		t.Fatalf("ClimateOff: %v", err)
	}
	hvac := climateAction(t, sender)
	if hvac.GetPowerOn() || hvac.GetManualOverride() {
		t.Errorf("ClimateOff encoded power_on=%v override=%v", hvac.GetPowerOn(), hvac.GetManualOverride())
	}
}
