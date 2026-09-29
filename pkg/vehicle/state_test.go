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

func TestGetStateDriveSendsGetDriveState(t *testing.T) {
	car, sender := newTestVehicle()
	sender.Listen(nil)
	sender.fixedResponse = &universal.RoutableMessage{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if _, err := car.GetState(ctx, StateCategoryDrive); err != nil {
		if errors.Is(err, protocol.ErrBLEStateLatencyFirmware) {
			t.Fatal("GetState must still send GetDriveState; BLE latency is firmware, not a client refusal")
		}
		t.Fatalf("GetState: %v", err)
	}
	if sender.lastMessage == nil {
		t.Fatal("dispatcher did not capture a request")
	}
	var action carserver.Action
	if err := proto.Unmarshal(sender.lastMessage.GetProtobufMessageAsBytes(), &action); err != nil {
		t.Fatalf("unmarshal Action: %v", err)
	}
	if action.GetVehicleAction().GetGetVehicleData().GetGetDriveState() == nil {
		t.Fatalf("request was not GetDriveState: %v", &action)
	}
}
