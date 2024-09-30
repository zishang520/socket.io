package adapter

import (
	"github.com/zishang520/socket.io/v2/socket"
)

type (
	Adapter = socket.Adapter

	SessionAwareAdapter = socket.SessionAwareAdapter

	// A cluster-ready adapter. Any extending interface must:
	//
	// - implement [ClusterAdapter.DoPublish] and [ClusterAdapter.DoPublishResponse]
	//
	// - call [ClusterAdapter.OnMessage] and [ClusterAdapter.OnResponse]
	ClusterAdapter interface {
		Adapter

		Uid() ServerId
		OnMessage(*ClusterMessage, Offset)
		OnResponse(*ClusterResponse)
		Publish(*ClusterMessage)
		PublishAndReturnOffset(*ClusterMessage) (Offset, error)
		DoPublish(*ClusterMessage) (Offset, error)
		PublishResponse(ServerId, *ClusterResponse)
		DoPublishResponse(ServerId, *ClusterResponse) error
	}

	ClusterAdapterWithHeartbeat interface {
		ClusterAdapter

		SetOpts(*ClusterAdapterOptions)
	}
)
