package main

import (
	"encoding/json"
	"fmt"
	"github.com/hyperledger/aries-framework-go/pkg/client/didexchange"
	"github.com/hyperledger/aries-framework-go/pkg/client/outofband"
	"github.com/hyperledger/aries-framework-go/pkg/didcomm/common/service"
	outofband2 "github.com/hyperledger/aries-framework-go/pkg/didcomm/protocol/outofband"
	"github.com/hyperledger/aries-framework-go/pkg/didcomm/transport/ws"
	"github.com/hyperledger/aries-framework-go/pkg/framework/aries"
	"net/http"
)

func main() {

	didExchangeClient, oobClient, err := createDIDClient(5000)
	if err != nil {
		fmt.Println("Failed to create DID Client for Router")
	}

	http.Handle("/invitation", &InvitationHandler{DIDExchangeClient: *didExchangeClient, OOBClient: *oobClient})
	http.ListenAndServe(":5000", nil)

}

type DIDExchangeClient struct {
	didexchange.Client
}

type OOBClient struct {
	outofband.Client
}

func createDIDClient(port int32) (*DIDExchangeClient, *OOBClient, error) {

	ngrokAddress := "8574-84-63-28-137.eu.ngrok.io"
	address := fmt.Sprintf("localhost:%d", port+1)
	inbound, err := ws.NewInbound(address, "ws://"+ngrokAddress, "", "")

	mediaTypeProfiles := []string{"didcomm/v2", "didcomm/aip2;env=rfc587", "didcomm/aip2;env=rfc19", "didcomm/aip1"}

	framework, err := aries.New(
		aries.WithInboundTransport(inbound),
		aries.WithOutboundTransports(ws.NewOutbound()),
		//aries.WithStoreProvider(mem.NewProvider()),
		//aries.WithProtocolStateStoreProvider(mem.NewProvider()),
		//aries.WithKeyType(kms.ED25519),
		//aries.WithKeyAgreementType(kms.NISTP384ECDHKW),
		aries.WithMediaTypeProfiles(mediaTypeProfiles),
		aries.WithTransportReturnRoute("all"),
	)
	if err != nil {
		fmt.Println("Failed to create framework")
	}

	ctx, err := framework.Context()
	if err != nil {
		fmt.Println("Failed to create framework context")
	}

	fmt.Println("Context created successfully")
	fmt.Println(ctx.ServiceEndpoint())

	// DID Exchange Client: create new Invitation
	didExchangeClient, err := didexchange.New(ctx)
	if err != nil {
		fmt.Println("Failed to create DIDExchange Client")
	}

	// Out Of Band Client
	outOfBandClient, err := outofband.New(ctx)
	if err != nil {
		fmt.Println("Failed to create OutOfBand Controller")
	}
	fmt.Println("Created Out Of Band Controller")

	// Route Exchange Client

	/*
		routeExchangeClient, err := mediator.New(ctx)
		if err != nil {
			fmt.Printf("Failed to create route client: %w", err)
		}
	*/

	events := make(chan service.DIDCommAction)

	// Register DIDExchange Client and Route Exchange client

	/*
		err = routeExchangeClient.RegisterActionEvent(events)
		if err != nil {
			fmt.Printf("Failed to register for action events on the routing client: %w", err)
		}
	*/

	err = didExchangeClient.RegisterActionEvent(events)
	if err != nil {
		fmt.Printf("Failed to register for action events on the DID Exchange client: %w", err)
	}

	go func() {
		service.AutoExecuteActionEvent(events)
	}()

	return &DIDExchangeClient{Client: *didExchangeClient}, &OOBClient{Client: *outOfBandClient}, nil
}

type InvitationHandler struct {
	DIDExchangeClient DIDExchangeClient
	OOBClient         OOBClient
}

func (handler *InvitationHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:

		// Router creates Route Request Message (Create actual request)

		/*
			routeRequest, err := json.Marshal(mediator.NewRequest())
			if err != nil {
				fmt.Println("Failed to create routeRequest")
			}
			fmt.Printf("Created Route Request %w", routeRequest)

		*/

		// Create DIDExchange request
		/*
			invitation, err := handler.DIDExchangeClient.CreateInvitation("Router Invitation")
			if err != nil {
				fmt.Printf("Failed to create new Invitation with DIDExchangeClient: %w", err)
			}
			fmt.Printf("Created DIDExchange Invitation")

				inv, err := json.Marshal(invitation)
				if err != nil {
					fmt.Printf("Failed to create Json from DIDEchange Invitation")
				}
		*/

		//invitation, err := handler.DIDClient.CreateInvitation("Router Invitation")
		oobInvitation, err := handler.OOBClient.CreateInvitation(
			nil,
			outofband.WithLabel("Router Invitation"),
			outofband.WithAccept(outofband2.MediaTypeProfileDIDCommV2, outofband2.MediaTypeProfileAIP2RFC587, outofband2.MediaTypeProfileDIDCommAIP2RFC19, outofband2.MediaTypeProfileDIDCommAIP1),
		)

		if err != nil {
			fmt.Printf("Failed to create Invitation from Router: %w", err)
		}

		fmt.Printf("Created Out of Band Invitation: %s", oobInvitation)

		response, err := json.Marshal(oobInvitation)
		if err != nil {
			fmt.Printf("Failed to get Response: %w", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.Write(response)
	}
}
