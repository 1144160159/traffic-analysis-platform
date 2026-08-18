package extractor

import (
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

var (
	ftpPASVReply = regexp.MustCompile(`(?i)(?:^|\n)\s*227[^\r\n]*\(([0-9]{1,3}),([0-9]{1,3}),([0-9]{1,3}),([0-9]{1,3}),([0-9]{1,3}),([0-9]{1,3})\)`)
	ftpEPSVReply = regexp.MustCompile(`(?i)(?:^|\n)\s*229[^\r\n]*\(\|\|\|([0-9]{1,5})\|\)`)
)

// CorrelateFTPPassiveData binds the selected data flow to the exact PASV/EPSV
// endpoint advertised on the approved control session. Either tuple ordering
// is accepted, but the advertised server endpoint must match exactly.
func CorrelateFTPPassiveData(clientControl, serverControl []byte, controlServerIP string, dataSourceIP string, dataSourcePort uint16, dataDestinationIP string, dataDestinationPort uint16) bool {
	client := strings.ToUpper(string(clientControl))
	server := string(serverControl)
	type endpoint struct {
		ip   string
		port uint16
	}
	var endpoints []endpoint
	if strings.Contains(client, "PASV\r\n") || strings.Contains(client, "PASV\n") {
		for _, match := range ftpPASVReply.FindAllStringSubmatch(server, -1) {
			parts := make([]int, 6)
			valid := true
			for index := range parts {
				parts[index], _ = strconv.Atoi(match[index+1])
				if parts[index] < 0 || parts[index] > 255 {
					valid = false
				}
			}
			if valid {
				endpoints = append(endpoints, endpoint{
					ip:   net.IPv4(byte(parts[0]), byte(parts[1]), byte(parts[2]), byte(parts[3])).String(),
					port: uint16(parts[4]*256 + parts[5]),
				})
			}
		}
	}
	if strings.Contains(client, "EPSV\r\n") || strings.Contains(client, "EPSV\n") {
		for _, match := range ftpEPSVReply.FindAllStringSubmatch(server, -1) {
			port, err := strconv.ParseUint(match[1], 10, 16)
			if err == nil && port > 0 {
				endpoints = append(endpoints, endpoint{ip: net.ParseIP(controlServerIP).String(), port: uint16(port)})
			}
		}
	}
	if len(endpoints) != 1 || endpoints[0].ip == "<nil>" || endpoints[0].port == 0 {
		return false
	}
	// FTPDataRequest is oriented client->server so ServerToClient remains the
	// only approved restoration byte direction.
	return net.ParseIP(dataDestinationIP).Equal(net.ParseIP(endpoints[0].ip)) && dataDestinationPort == endpoints[0].port
}

func ftpLineIndex(control, prefix string) int {
	control = strings.ReplaceAll(control, "\r\n", "\n")
	for index, line := range strings.Split(control, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(trimmed), prefix) {
			return index
		}
	}
	return -1
}

func ftpCommandArgument(control, command string) string {
	control = strings.ReplaceAll(control, "\r\n", "\n")
	for _, line := range strings.Split(control, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(trimmed), command+" ") {
			return strings.TrimSpace(trimmed[len(command)+1:])
		}
	}
	return ""
}

func extractFTPPassive(input Input, limits Limits) Result {
	result := baseResult(ProfileFTPPassive, "ftp-passive-retr")
	if input.FTPTLSEnabled {
		result.Status = StatusUnsupported
		result.StatusReason = "FTP TLS control or data is excluded"
		return result
	}
	if status, missing, truncation, unsafe := combinedStreamStatus(input.ClientToServer, input.ServerToClient); unsafe {
		result.Status = status
		result.StatusReason = "FTP control stream is incomplete or unsafe"
		result.MissingRanges = missing
		result.TruncationOffset = truncation
		return result
	}
	client := string(input.ClientToServer.Bytes)
	server := string(input.ServerToClient.Bytes)
	pasv := ftpLineIndex(client, "PASV")
	if pasv < 0 {
		pasv = ftpLineIndex(client, "EPSV")
	}
	retr := ftpLineIndex(client, "RETR ")
	if pasv < 0 || retr < 0 || pasv >= retr {
		result.Status = StatusUnsupported
		result.StatusReason = "FTP control session lacks passive mode followed by RETR"
		return result
	}
	if input.FTPDataConnections != 1 || !input.FTPDataCorrelated {
		result.Status = StatusUnsupported
		result.StatusReason = "FTP data connection is absent or ambiguous"
		return result
	}
	if input.FTPDataReset || !input.FTPDataBoundaryComplete {
		result.Status = StatusTruncated
		result.StatusReason = "FTP data connection lacks exact SYN-to-FIN sequence coverage or ended with reset"
		return result
	}
	reply150 := ftpLineIndex(server, "150")
	reply226 := ftpLineIndex(server, "226")
	passiveReply := ftpLineIndex(server, "227")
	if ftpLineIndex(client, "EPSV") >= 0 {
		passiveReply = ftpLineIndex(server, "229")
	}
	if passiveReply < 0 || passiveReply >= reply150 {
		result.Status = StatusUnsupported
		result.StatusReason = "FTP passive command lacks a matching ordered passive endpoint reply"
		return result
	}
	if reply150 < 0 {
		result.Status = StatusTruncated
		result.StatusReason = "FTP RETR lacks 150 transfer-start reply"
		return result
	}
	if reply226 < 0 || reply226 <= reply150 {
		result.Status = StatusTruncated
		result.StatusReason = "FTP data transfer lacks ordered 226 completion reply"
		return result
	}
	streamStatus, terminal := statusFromStream(input.FTPDataServerToClient)
	if terminal || streamStatus == StatusPartial {
		result.Status = streamStatus
		result.StatusReason = "FTP data stream is incomplete or unsafe"
		result.MissingRanges = input.FTPDataServerToClient.MissingRanges
		result.TruncationOffset = input.FTPDataServerToClient.TruncationAt
		if streamStatus == StatusPartial || streamStatus == StatusTruncated {
			return finalize(result, input.FTPDataServerToClient.Bytes, input.FTPDataServerToClient.Bytes, limits)
		}
		return result
	}
	filename := ftpCommandArgument(client, "RETR")
	result.WireFilename = filename
	result.SanitizedFilename = SanitizeFilename(filename)
	result.DeclaredMIMEType = "application/octet-stream"
	result.DetectedMIMEType = http.DetectContentType(input.FTPDataServerToClient.Bytes)
	result.Status = StatusComplete
	result.StatusReason = "approved passive FTP RETR completed with 150 and 226 replies"
	return finalize(result, input.FTPDataServerToClient.Bytes, input.FTPDataServerToClient.Bytes, limits)
}
