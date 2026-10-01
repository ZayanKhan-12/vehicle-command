package vehicle

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/teslamotors/vehicle-command/pkg/protocol"
	carserver "github.com/teslamotors/vehicle-command/pkg/protocol/protobuf/carserver"
	universal "github.com/teslamotors/vehicle-command/pkg/protocol/protobuf/universalmessage"
)

func TestSetChargingAmpsSendsRequestedValueBelowFive(t *testing.T) {
	for _, amps := range []int32{0, 1, 2, 5} {
		car, sender := newTestVehicle()
		sender.Listen(nil)
		sender.fixedResponse = &universal.RoutableMessage{}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		err := car.SetChargingAmps(ctx, amps)
		cancel()
		if errors.Is(err, protocol.ErrChargingAmpsBelowFloorFirmware) {
			t.Fatalf("SetChargingAmps(%d) must still send the requested integer", amps)
		}
		if err != nil {
			t.Fatalf("SetChargingAmps(%d): %v", amps, err)
		}
		if sender.lastMessage == nil {
			t.Fatal("dispatcher did not capture a request")
		}
		var action carserver.Action
		if err := proto.Unmarshal(sender.lastMessage.GetProtobufMessageAsBytes(), &action); err != nil {
			t.Fatalf("unmarshal Action: %v", err)
		}
		got := action.GetVehicleAction().GetSetChargingAmpsAction()
		if got == nil || got.GetChargingAmps() != amps {
			t.Fatalf("charging_amps = %v, want %d", got, amps)
		}
	}
}
