package main

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/hyperledger/aries-framework-go/component/storageutil/mem"
	"github.com/hyperledger/aries-framework-go/pkg/client/didexchange"
	"github.com/hyperledger/aries-framework-go/pkg/client/mediator"
	"github.com/hyperledger/aries-framework-go/pkg/client/outofbandv2"
	"github.com/hyperledger/aries-framework-go/pkg/didcomm/common/peerdid"
	"github.com/hyperledger/aries-framework-go/pkg/didcomm/common/service"
	"github.com/hyperledger/aries-framework-go/pkg/didcomm/packer"
	"github.com/hyperledger/aries-framework-go/pkg/didcomm/packer/anoncrypt"
	"github.com/hyperledger/aries-framework-go/pkg/didcomm/packer/authcrypt"
	"github.com/hyperledger/aries-framework-go/pkg/didcomm/protocol/decorator"
	"github.com/hyperledger/aries-framework-go/pkg/didcomm/transport"
	"github.com/hyperledger/aries-framework-go/pkg/didcomm/transport/ws"
	"github.com/hyperledger/aries-framework-go/pkg/doc/jose"
	"github.com/hyperledger/aries-framework-go/pkg/framework/aries"
	"github.com/hyperledger/aries-framework-go/pkg/framework/context"
	"github.com/hyperledger/aries-framework-go/pkg/kms"
	"net/http"
)

func main() {

	didExchangeClient, oobClient, ctx, err := createDIDClient(5000)
	if err != nil {
		panic(err)
	}

	http.Handle("/invitation", &InvitationHandler{DIDExchangeClient: *didExchangeClient, OOBClient: *oobClient, Provider: *ctx})

	//http.Handle("/invitation", &InvitationHandler{DIDExchangeClient: *didExchangeClient, OOBV2Client: *oobClient})
	http.ListenAndServe(":5000", nil)

}

type DIDExchangeClient struct {
	didexchange.Client
}

type OOBV2Client struct {
	outofbandv2.Client
}

type InvitationHandler struct {
	DIDExchangeClient DIDExchangeClient
	OOBClient         OOBV2Client
	Provider          context.Provider
}

func createDIDClient(port int32) (*DIDExchangeClient, *OOBV2Client, *context.Provider, error) {

	ngrokAddress := "1c24-84-63-28-137.eu.ngrok.io"
	address := fmt.Sprintf("localhost:%d", port+1)
	inbound, err := ws.NewInbound(address, "ws://"+ngrokAddress, "", "")

	//mediaTypeProfiles := []string{"didcomm/v2", "didcomm/aip2;env=rfc587", "didcomm/aip2;env=rfc19", "didcomm/aip1"}

	// Router Setup
	framework, err := aries.New(
		aries.WithInboundTransport(inbound),
		aries.WithOutboundTransports(ws.NewOutbound()),
		aries.WithTransportReturnRoute("all"),
		aries.WithMediaTypeProfiles([]string{transport.MediaTypeDIDCommV2Profile}),
		aries.WithKeyAgreementType(kms.NISTP521ECDHKWType),
		aries.WithStoreProvider(mem.NewProvider()),
		aries.WithProtocolStateStoreProvider(mem.NewProvider()),
		aries.WithPacker(
			func(prov packer.Provider) (packer.Packer, error) {
				return authcrypt.New(prov, jose.A256CBCHS512)
			},
			func(prov packer.Provider) (packer.Packer, error) {
				return anoncrypt.New(prov, jose.A256GCM)
			},
		),
	)
	if err != nil {
		panic(err)
	}

	ctx, err := framework.Context()
	if err != nil {
		panic("Failed to create framework context")
	}

	fmt.Println("Context created successfully")
	fmt.Println(ctx.ServiceEndpoint())

	// DID Exchange Client: create new Invitation
	routerDIDs, err := didexchange.New(ctx)
	if err != nil {
		panic(err)
	}

	// Create router Client
	routerClient, err := mediator.New(ctx)
	if err != nil {
		panic(err)
	}

	// Out Of Band 2 Client
	outOfBandv2Client, err := outofbandv2.New(ctx)
	if err != nil {
		panic(err)
	}
	fmt.Println("Created Out Of Band Controller")

	// Register DIDs and Route Exchange client
	events := make(chan service.DIDCommAction)
	err = routerDIDs.RegisterActionEvent(events)
	if err != nil {
		panic(err)
	}

	err = routerClient.RegisterActionEvent(events)
	if err != nil {
		panic(err)
	}

	go func() {
		service.AutoExecuteActionEvent(events)
	}()

	return &DIDExchangeClient{Client: *routerDIDs}, &OOBV2Client{Client: *outOfBandv2Client}, ctx, nil
}

func (handler *InvitationHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:

		// Create route-request message
		//routeRequest, err := json.Marshal(mediator.NewRequest())
		//routeRequest := mediator.NewRequest()
		inv, err := handler.DIDExchangeClient.CreateInvitation("Router Invitation")
		if err != nil {
			panic(err)
		}
		fmt.Println(inv.ServiceEndpoint)

		peerDID := peerdid.New(&handler.Provider)
		didv2, err := peerDID.CreatePeerDIDV2()
		if err != nil {
			return
		}

		oobInvitation, err := handler.OOBClient.CreateInvitation(
			outofbandv2.WithFrom(didv2.ID),
			outofbandv2.WithGoal("connect", "connect"),
			outofbandv2.WithAccept("didcomm/aip2;env=rfc19", transport.MediaTypeDIDCommV2Profile),
			outofbandv2.WithAttachments(&decorator.AttachmentV2{
				ID:          uuid.New().String(),
				Description: "connect",
				Data: decorator.AttachmentData{
					JSON: inv,
				},
			}),
		)

		if err != nil {
			panic(err)
		}

		fmt.Printf("Created Out of Band Invitation \n")

		res, err := handler.OOBClient.AcceptInvitation(oobInvitation)
		if err != nil {
			panic(err)
		}
		fmt.Println(res)

		conn, err := handler.DIDExchangeClient.GetConnection(res)
		fmt.Println(conn.RecipientKeys)

		response, err := json.Marshal(oobInvitation)
		if err != nil {
			panic(err)
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.Write(response)
	}
}
