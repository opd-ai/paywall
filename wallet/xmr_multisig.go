package wallet

import (
	"fmt"

	monero "github.com/monero-ecosystem/go-monero-rpc-client/wallet"
)

// MoneroMultisigState represents the current state of a Monero multisig wallet
type MoneroMultisigState struct {
	IsMultisig       bool     `json:"is_multisig"`
	IsReady          bool     `json:"is_ready"`
	Threshold        int      `json:"threshold"`         // m in m-of-n
	Total            int      `json:"total"`             // n in m-of-n
	MultisigInfo     string   `json:"multisig_info"`     // Prepared multisig info
	ParticipantInfos []string `json:"participant_infos"` // Info from all participants for MakeMultisig
	Address          string   `json:"address"`           // Multisig address if ready
}

// PrepareMultisig initializes multisig on a Monero wallet.
// This is the first step in the Monero multisig setup process and must be called
// on each participant's wallet before they can create a shared multisig wallet.
//
// Parameters:
//   - threshold: Number of required signatures (m in m-of-n)
//
// Returns:
//   - string: Multisig info to share with other participants
//   - error: If preparation fails
//
// Workflow:
//  1. Each participant calls PrepareMultisig() on their wallet
//  2. Each participant shares their multisig info with all others
//  3. All participants call MakeMultisig() with the collected info
//
// Example:
//
//	info, err := wallet.PrepareMultisig(2) // For 2-of-3 multisig
//
// Related: MakeMultisig, FinalizeMultisig
func (w *MoneroHDWallet) PrepareMultisig(threshold int) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if threshold < 2 {
		return "", fmt.Errorf("threshold must be at least 2 for multisig")
	}

	if w.client == nil {
		return "", fmt.Errorf("wallet client not initialized")
	}

	// Call Monero RPC prepare_multisig
	resp, err := w.client.PrepareMultisig()
	if err != nil {
		return "", fmt.Errorf("prepare multisig failed: %w", err)
	}

	return resp.MultisigInfo, nil
}

// MakeMultisig creates a multisig wallet from prepared multisig info.
// This is the second step in the Monero multisig setup process and combines
// the multisig info from all participants to create the shared wallet.
//
// Parameters:
//   - participantInfos: Multisig info strings from all OTHER participants (not including self)
//   - threshold: Number of required signatures (m in m-of-n)
//
// Returns:
//   - string: The multisig wallet address
//   - string: Additional multisig info for finalization (N-1/N and M/N only need this)
//   - error: If creation fails
//
// Workflow:
//  1. Collect multisig info from all participants (from PrepareMultisig)
//  2. Each participant calls MakeMultisig with the others' info
//  3. For N/N multisig, setup is complete
//  4. For M/N (M < N) multisig, proceed to ExportMultisigInfo/ImportMultisigInfo/FinalizeMultisig
//
// Example:
//
//	addr, info, err := wallet.MakeMultisig([]string{info1, info2}, 2)
//
// Related: PrepareMultisig, ExportMultisigInfo, ImportMultisigInfo, FinalizeMultisig
func (w *MoneroHDWallet) MakeMultisig(participantInfos []string, threshold int) (string, string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(participantInfos) == 0 {
		return "", "", fmt.Errorf("participant infos cannot be empty")
	}

	if threshold < 2 {
		return "", "", fmt.Errorf("threshold must be at least 2 for multisig")
	}

	total := len(participantInfos) + 1 // +1 for self
	if threshold > total {
		return "", "", fmt.Errorf("threshold (%d) cannot exceed total participants (%d)", threshold, total)
	}

	if w.client == nil {
		return "", "", fmt.Errorf("wallet client not initialized")
	}

	// Call Monero RPC make_multisig
	resp, err := w.client.MakeMultisig(&monero.RequestMakeMultisig{
		MultisigInfo: participantInfos,
		Threshold:    uint64(threshold),
	})
	if err != nil {
		return "", "", fmt.Errorf("make multisig failed: %w", err)
	}

	// Store the multisig address and config in the wallet
	w.multisigAddress = resp.Address
	w.multisigConfig = &MultisigConfig{
		Enabled:      true,
		RequiredSigs: threshold,
		TotalSigners: total,
		MultisigInfo: resp.MultisigInfo,
	}

	return resp.Address, resp.MultisigInfo, nil
}

// ExportMultisigInfo exports the current wallet's multisig synchronization data.
// This is required for M-of-N multisig wallets (where M < N) after MakeMultisig.
// Each participant must export their info and share it with all others.
//
// Returns:
//   - string: Multisig info to share with other participants
//   - error: If export fails
//
// Workflow (for M/N where M < N):
//  1. Each participant calls ExportMultisigInfo()
//  2. Each participant shares their info with all others
//  3. Each participant calls ImportMultisigInfo() with others' info
//  4. Each participant calls FinalizeMultisig()
//  5. Repeat steps 1-4 until all participants are synchronized
//
// Example:
//
//	info, err := wallet.ExportMultisigInfo()
//
// Related: ImportMultisigInfo, FinalizeMultisig
func (w *MoneroHDWallet) ExportMultisigInfo() (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.client == nil {
		return "", fmt.Errorf("wallet client not initialized")
	}

	// Call Monero RPC export_multisig_info
	resp, err := w.client.ExportMultisigInfo()
	if err != nil {
		return "", fmt.Errorf("export multisig info failed: %w", err)
	}

	return resp.Info, nil
}

// ImportMultisigInfo imports multisig synchronization data from other participants.
// This is required for M-of-N multisig wallets (where M < N) to synchronize
// the shared wallet state across all participants.
//
// Parameters:
//   - participantInfos: Multisig info strings from all OTHER participants
//
// Returns:
//   - int: Number of outputs imported
//   - error: If import fails
//
// Workflow (for M/N where M < N):
//  1. Collect ExportMultisigInfo() output from all other participants
//  2. Call ImportMultisigInfo() with the collected data
//  3. Call FinalizeMultisig() to complete synchronization
//  4. Repeat until all participants report is_ready = true
//
// Example:
//
//	n, err := wallet.ImportMultisigInfo([]string{info1, info2})
//
// Related: ExportMultisigInfo, FinalizeMultisig
func (w *MoneroHDWallet) ImportMultisigInfo(participantInfos []string) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(participantInfos) == 0 {
		return 0, fmt.Errorf("participant infos cannot be empty")
	}

	if w.client == nil {
		return 0, fmt.Errorf("wallet client not initialized")
	}

	// Call Monero RPC import_multisig_info
	resp, err := w.client.ImportMultisigInfo(&monero.RequestImportMultisigInfo{
		Info: participantInfos,
	})
	if err != nil {
		return 0, fmt.Errorf("import multisig info failed: %w", err)
	}

	return int(resp.NOutputs), nil
}

// FinalizeMultisig completes the multisig wallet setup after importing info.
// This is the final step for M-of-N multisig wallets (where M < N) to ensure
// all participants are synchronized and ready to use the shared wallet.
//
// Parameters:
//   - participantInfos: Final multisig info strings from all OTHER participants
//
// Returns:
//   - string: The finalized multisig wallet address
//   - error: If finalization fails
//
// Workflow (for M/N where M < N):
//  1. After ImportMultisigInfo(), collect final ExportMultisigInfo() from all participants
//  2. Each participant calls FinalizeMultisig() with the collected data
//  3. Verify IsMultisigReady() returns true
//  4. Wallet is ready for transactions
//
// Example:
//
//	addr, err := wallet.FinalizeMultisig([]string{info1, info2})
//
// Related: ExportMultisigInfo, ImportMultisigInfo, IsMultisigReady
func (w *MoneroHDWallet) FinalizeMultisig(participantInfos []string) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(participantInfos) == 0 {
		return "", fmt.Errorf("participant infos cannot be empty")
	}

	if w.client == nil {
		return "", fmt.Errorf("wallet client not initialized")
	}

	// Call Monero RPC finalize_multisig
	resp, err := w.client.FinalizeMultisig(&monero.RequestFinalizeMultisig{
		MultisigInfo: participantInfos,
	})
	if err != nil {
		return "", fmt.Errorf("finalize multisig failed: %w", err)
	}

	// Update the stored multisig address
	w.multisigAddress = resp.Address

	return resp.Address, nil
}

// IsMultisigWallet checks if the wallet is configured as multisig.
//
// Returns:
//   - bool: True if wallet is multisig
//   - error: If query fails
//
// Example:
//
//	isMultisig, err := wallet.IsMultisigWallet()
//
// Related: IsMultisigReady, GetMultisigState
func (w *MoneroHDWallet) IsMultisigWallet() (bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.client == nil {
		return false, fmt.Errorf("wallet client not initialized")
	}

	resp, err := w.client.IsMultisig()
	if err != nil {
		return false, fmt.Errorf("is multisig check failed: %w", err)
	}

	return resp.Multisig, nil
}

// IsMultisigReady checks if the multisig wallet is fully synchronized and ready.
//
// Returns:
//   - bool: True if wallet is ready for transactions
//   - error: If query fails
//
// Example:
//
//	ready, err := wallet.IsMultisigReady()
//
// Related: IsMultisigWallet, GetMultisigState
func (w *MoneroHDWallet) IsMultisigReady() (bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.client == nil {
		return false, fmt.Errorf("wallet client not initialized")
	}

	resp, err := w.client.IsMultisig()
	if err != nil {
		return false, fmt.Errorf("is multisig check failed: %w", err)
	}

	return resp.Ready, nil
}

// GetMultisigState returns the current multisig configuration and state.
//
// Returns:
//   - *MoneroMultisigState: Current multisig state
//   - error: If query fails
//
// Example:
//
//	state, err := wallet.GetMultisigState()
//	if state.IsReady {
//	    // Wallet is ready for transactions
//	}
//
// Related: IsMultisigWallet, IsMultisigReady
func (w *MoneroHDWallet) GetMultisigState() (*MoneroMultisigState, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.client == nil {
		return nil, fmt.Errorf("wallet client not initialized")
	}

	resp, err := w.client.IsMultisig()
	if err != nil {
		return nil, fmt.Errorf("get multisig state failed: %w", err)
	}

	state := &MoneroMultisigState{
		IsMultisig: resp.Multisig,
		IsReady:    resp.Ready,
		Threshold:  int(resp.Threshold),
		Total:      int(resp.Total),
	}

	return state, nil
}

// SignMultisigTransaction signs a multisig transaction with the current participant's key.
// This is used for spending funds from a multisig wallet.
//
// Parameters:
//   - txSet: The transaction set string from InitiateMultisigTransaction or a previous SignMultisigTransaction
//
// Returns:
//   - string: The signed transaction set (pass to next signer or broadcast if complete)
//   - error: If signing fails
//
// Workflow:
//  1. Initiator creates transaction with TransferSplit (returns tx_blob)
//  2. Each required participant calls SignMultisigTransaction(tx_blob)
//  3. After M signatures, use SubmitMultisig() to broadcast
//
// Example:
//
//	signedTx, err := wallet.SignMultisigTransaction(txSet)
//
// Related: SubmitMultisig
func (w *MoneroHDWallet) SignMultisigTransaction(txSet string) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if txSet == "" {
		return "", fmt.Errorf("transaction set cannot be empty")
	}

	if w.client == nil {
		return "", fmt.Errorf("wallet client not initialized")
	}

	// Call Monero RPC sign_multisig
	resp, err := w.client.SignMultisig(&monero.RequestSignMultisig{
		TxDataHex: txSet,
	})
	if err != nil {
		return "", fmt.Errorf("sign multisig transaction failed: %w", err)
	}

	return resp.TxDataHex, nil
}

// SubmitMultisig submits a fully-signed multisig transaction to the network.
// This is the final step after collecting all required signatures.
//
// Parameters:
//   - txHex: The fully-signed transaction hex from SignMultisigTransaction
//
// Returns:
//   - []string: Transaction IDs of the submitted transactions
//   - error: If submission fails
//
// Workflow:
//  1. Collect M signatures via SignMultisigTransaction
//  2. Last signer calls SubmitMultisig with the fully-signed tx
//  3. Transaction is broadcast to the network
//
// Example:
//
//	txIDs, err := wallet.SubmitMultisig(fullySignedTx)
//
// Related: SignMultisigTransaction
func (w *MoneroHDWallet) SubmitMultisig(txHex string) ([]string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if txHex == "" {
		return nil, fmt.Errorf("transaction hex cannot be empty")
	}

	if w.client == nil {
		return nil, fmt.Errorf("wallet client not initialized")
	}

	// Call Monero RPC submit_multisig
	resp, err := w.client.SubmitMultisig(&monero.RequestSubmitMultisig{
		TxDataHex: txHex,
	})
	if err != nil {
		return nil, fmt.Errorf("submit multisig transaction failed: %w", err)
	}

	return resp.TxHashList, nil
}
