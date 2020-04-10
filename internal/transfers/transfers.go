// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package transfers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/moov-io/paygate/internal/model"
	"github.com/moov-io/paygate/pkg/id"

	"github.com/go-kit/kit/log"
)

// getTransferObjects performs database lookups to grab all the objects needed to make a transfer.
//
// This method also verifies the status of the Receiver, Receiver Depository and Originator Repository
//
// All return values are either nil or non-nil and the error will be the opposite.
func (c *TransferRouter) getTransferObjects(
	userID id.User,
	origID model.OriginatorID,
	origDepID id.Depository,
	recID model.ReceiverID,
	recDepID id.Depository,
) (*model.Receiver, *model.Depository, *model.Originator, *model.Depository, error) {
	// Receiver
	receiver, err := c.receiverRepository.GetUserReceiver(recID, userID)
	if receiver == nil || err != nil {
		return nil, nil, nil, nil, fmt.Errorf("receiver not found: %v", err)
	}
	if err := receiver.Validate(); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("receiver: %v", err)
	}

	receiverDep, err := c.depRepo.GetUserDepository(recDepID, userID)
	if receiverDep == nil || err != nil {
		return nil, nil, nil, nil, fmt.Errorf("receiver depository not found: %v", err)
	}
	if err := receiverDep.Validate(); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("receiver depository: %v", err)
	}
	if receiverDep.Status != model.DepositoryVerified {
		return nil, nil, nil, nil, fmt.Errorf("receiver depository %s is in status %v", receiverDep.ID, receiverDep.Status)
	}

	// Originator
	orig, err := c.origRepo.GetUserOriginator(origID, userID)
	if orig == nil || err != nil {
		return nil, nil, nil, nil, fmt.Errorf("originator not found: %v", err)
	}
	if err := orig.Validate(); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("originator: %v", err)
	}

	origDep, err := c.depRepo.GetUserDepository(origDepID, userID)
	if origDep == nil || err != nil {
		return nil, nil, nil, nil, fmt.Errorf("originator Depository not found: %v", err)
	}
	if err := origDep.Validate(); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("originator depository: %v", err)
	}
	if origDep.Status != model.DepositoryVerified {
		return nil, nil, nil, nil, fmt.Errorf("originator Depository %s is in status %v", origDep.ID, origDep.Status)
	}

	return receiver, receiverDep, orig, origDep, nil
}

func writeResponse(logger log.Logger, w http.ResponseWriter, reqCount int, transfers []*model.Transfer) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	if reqCount == 1 {
		// don't render surrounding array for single transfer create
		// (it's coming from POST /transfers, not POST /transfers/batch)
		json.NewEncoder(w).Encode(transfers[0])
	} else {
		json.NewEncoder(w).Encode(transfers)
	}
}
