package adapter

import (
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

// SocketDetailsToResponses converts socket details to their wire representation.
func SocketDetailsToResponses(localSockets []socket.SocketDetails) []SocketResponse {
	responses := make([]SocketResponse, len(localSockets))
	for i, client := range localSockets {
		var rooms []socket.Room
		if clientRooms := client.Rooms(); clientRooms != nil {
			rooms = clientRooms.Keys()
		}
		responses[i] = SocketResponse{
			Id:        client.Id(),
			Handshake: client.Handshake(),
			Rooms:     utils.NonNilSlice(rooms),
			Data:      client.Data(),
		}
	}
	return responses
}

// SocketResponsesToDetailsAny converts wire responses to socket details stored as any values.
func SocketResponsesToDetailsAny(socketResponses []SocketResponse) []any {
	responses := make([]any, len(socketResponses))
	for i := range socketResponses {
		responses[i] = NewRemoteSocket(&socketResponses[i])
	}
	return responses
}

// SocketDetailsToAny converts socket details to any values without changing the underlying objects.
func SocketDetailsToAny(localSockets []socket.SocketDetails) []any {
	responses := make([]any, len(localSockets))
	for i, client := range localSockets {
		responses[i] = client
	}
	return responses
}

// AnySliceToSocketDetails converts any values to socket details.
func AnySliceToSocketDetails(data []any) []socket.SocketDetails {
	responses := make([]socket.SocketDetails, len(data))
	for i, item := range data {
		responses[i] = utils.TryCast[socket.SocketDetails](item)
	}
	return responses
}
