package adapter

import (
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

func socketDetailsToResponses(localSockets []socket.SocketDetails) []SocketResponse {
	responses := make([]SocketResponse, len(localSockets))
	for i, client := range localSockets {
		responses[i] = SocketResponse{
			Id:        client.Id(),
			Handshake: client.Handshake(),
			Rooms:     client.Rooms().Keys(),
			Data:      client.Data(),
		}
	}
	return responses
}

func socketResponsesToDetailsAny(socketResponses []SocketResponse) []any {
	responses := make([]any, len(socketResponses))
	for i := range socketResponses {
		responses[i] = NewRemoteSocket(&socketResponses[i])
	}
	return responses
}

func socketDetailsToAny(localSockets []socket.SocketDetails) []any {
	responses := make([]any, len(localSockets))
	for i, client := range localSockets {
		responses[i] = client
	}
	return responses
}

func anySliceToSocketDetails(data []any) []socket.SocketDetails {
	responses := make([]socket.SocketDetails, len(data))
	for i, item := range data {
		responses[i] = utils.TryCast[socket.SocketDetails](item)
	}
	return responses
}
