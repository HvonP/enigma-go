package enigma

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"sync"
)

type (
	trafficLogRow struct {
		Sent     json.RawMessage `json:"Sent,omitempty"`
		Received json.RawMessage `json:"Received,omitempty"`
	}

	fileTrafficLog struct {
		FileName string
		Messages []trafficLogRow
		mutex    sync.Mutex
	}
)

// Opened implements the TrafficLogger interface
func (t *fileTrafficLog) Opened() {

}

// Sent implements the TrafficLogger interface
func (t *fileTrafficLog) Sent(message []byte) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.Messages = append(t.Messages, trafficLogRow{Sent: message})
}

// Received implements the TrafficLogger interface
func (t *fileTrafficLog) Received(message []byte) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.Messages = append(t.Messages, trafficLogRow{Received: message})
}

// Closed implements the TrafficLogger interface
func (t *fileTrafficLog) Closed() {
	bytes, _ := json.MarshalIndent(t.Messages, "", "\t")
	ioutil.WriteFile(t.FileName, bytes, 0644)
}

func newFileTrafficLogger(filename string) *fileTrafficLog {
	return &fileTrafficLog{FileName: filename, Messages: make([]trafficLogRow, 0, 1000)}
}

func readTrafficLog(fileName string) []trafficLogRow {
	file, _ := ioutil.ReadFile(fileName)
	result := []trafficLogRow{}
	_ = json.Unmarshal(file, &result)
	return result
}

//TODO: Naming convention is a bit off here. Hard to interpret
type trafficEntry struct {
	Direction    string
	SentHash     string
	ReceivedHash string
	ID           int
}

//TODO: Naming convention is a bit off here. Hard to interpret
type trafficMap struct {
	Traffic []trafficEntry
}

func (traffic *trafficMap) AddTraffic(item trafficEntry) []trafficEntry {
	traffic.Traffic = append(traffic.Traffic, item)
	return traffic.Traffic
}

func (tMap *trafficMap) hashTrafficLog(fileName string) {
	file, _ := ioutil.ReadFile(fileName)
	result := []trafficLogRow{}
	_ = json.Unmarshal(file, &result)

	type Message struct {
		Jsonrpc string      `json:"jsonrpc"`
		Method  string      `json:"method"`
		Delta   string      `json:"delta"`
		Handle  int         `json:"handle"`
		ID      int         `json:"id"`
		Result  interface{} `json:"result"`
	}

	for _, m := range result {
		if m.Sent != nil {
			var msg Message
			json.Unmarshal([]byte(m.Sent), &msg)
			//json, _ := json.Marshal(&m.Sent)
			//sha1 := hashString(string(json))
			//Ignoring params which is not needed right?
			/*
				var params string
				if len(msg.Params) > 0 {
					params = fmt.Sprint(msg.Params[0])
				} else {
					params = ""
				}
			*/
			hash := fmt.Sprintf("%s%s%s%d", msg.Jsonrpc, msg.Method, msg.Delta, msg.Handle)
			sha1 := hashString(hash)
			//fmt.Printf("SHA1 of the sent message is with specific fields only is : %s \n", sha1)
			entry := trafficEntry{Direction: "Sent", SentHash: sha1, ReceivedHash: "", ID: msg.ID}
			//trafficMap.AddTraffic(entry)
			tMap.AddTraffic(entry)
		} else if m.Received != nil {
			var msg Message
			json.Unmarshal([]byte(m.Received), &msg)
			//fmt.Printf("Parsed message received ID is: %d \n", msg.ID)
			//fmt.Printf("Parsed message received Method is: %s \n", msg.Method)

			//Get the Hash of the Sent message
			var found = false
			//for _, v := range trafficMap.Traffic {
			for _, v := range tMap.Traffic {
				if v.ID == msg.ID {
					found = true
					entry := trafficEntry{Direction: "Received", SentHash: v.SentHash, ID: msg.ID}
					//trafficMap.AddTraffic(entry)
					tMap.AddTraffic(entry)
				}
			}
			if !found {
				//entry := trafficEntry{Direction: "Received", SentHash: "", ReceivedHash: sha1, ID: msg.ID}
				//tMap.AddTraffic(entry)
				fmt.Printf("Nohthing to add as the ID is likely zero, %d\n", msg.ID)
			}

		}
	}

	//Mainly for printout reasons

	for _, m := range tMap.Traffic {
		fmt.Printf("SentHash: %s, ID: %d\n", m.SentHash, m.ID)
	}

	//If Sent, for each message generate hash w/o the id.
	//If Received, and an id is present, then map which sent message + hash it links to.

	//return nil

	fmt.Printf("Hashed traffic log\n")
}

func (tMap *trafficMap) liveTrafficLog(sentMessage json.RawMessage, wg *sync.WaitGroup) {

	defer wg.Done()

	type Message struct {
		Jsonrpc string      `json:"jsonrpc"`
		Method  string      `json:"method"`
		Delta   string      `json:"delta"`
		Handle  int         `json:"handle"`
		ID      int         `json:"id"`
		Result  interface{} `json:"result"`
	}

	var msg Message
	json.Unmarshal([]byte(sentMessage), &msg)

	sha1 := hashString(fmt.Sprintf("%s%s%s%d", msg.Jsonrpc, msg.Method, msg.Delta, msg.Handle))

	//If the current hash is found then replace the existing ID instead
	var found = false
	for k, v := range tMap.Traffic {
		if v.SentHash == sha1 {
			found = true
			//fmt.Printf("Entry already exists with id: %d, replacing with new id: %d\n", v.ID, msg.ID)

			//Replace current ID to be new one
			tMap.Traffic[k].ID = msg.ID
		}
	}
	if !found {
		entry := trafficEntry{Direction: "Sent", SentHash: sha1, ReceivedHash: "", ID: msg.ID}
		tMap.AddTraffic(entry)
		//fmt.Printf("Entry did not exist with that hash. Adding a new entry with id: %d\n", msg.ID)
	}

	//fmt.Printf("Sha1 of sent message with method: %s is %s\n", msg.Method, sha1)
}

func ensureCorrectID(livemessage json.RawMessage, liveMap trafficMap, hashMap trafficMap, wg *sync.WaitGroup) {

	defer wg.Done()

	// Parse livemessage and hash it
	type Message struct {
		Jsonrpc string      `json:"jsonrpc"`
		Method  string      `json:"method"`
		Delta   string      `json:"delta"`
		Handle  int         `json:"handle"`
		ID      int         `json:"id"`
		Result  interface{} `json:"result"`
	}

	var msg Message
	json.Unmarshal([]byte(livemessage), &msg)

	// Only do this if the ID of the message is actually set
	if msg.ID > 0 {

		fmt.Printf("Searching for with id: %d\n", msg.ID)
		//If the current hash is found then replace the existing ID instead
		var sentHash string
		var found = false
		for _, v := range hashMap.Traffic {
			//fmt.Printf("v.ReceivedHash: %s\n", v.ReceivedHash)
			if v.ID == msg.ID {
				found = true
				//fmt.Printf("Found the hash in the hashmap\n")
				sentHash = v.SentHash
				//Break after the first hit?

			}
		}

		if !found {
			// Throw some error as the message is not found at all
			//fmt.Printf("MISSING MESSAGE\n")
			//fmt.Printf("MISSING MESSAGE, ID: %d\n", msg.ID)
			//fmt.Printf("MISSING MESSAGE, Result: %s\n ", resultstring)
			fmt.Printf("ERROR: MESSAGE NOT EXPECTED AT ALL, with id: %d and senthash: %s\n", msg.ID, sentHash)
		}
		// So here we have the sentHash.. lets find the correct ID via the liveMap
		if found {
			// Find the sentMessageHash via the hashMap
			var foundSent = false
			fmt.Printf("Searching for senthash: %s and id: %d\n", sentHash, msg.ID)
			for _, y := range liveMap.Traffic {
				fmt.Printf("v.SentHash: %s\n", y.SentHash)
				if y.SentHash == sentHash {
					foundSent = true
					//fmt.Printf("Found the senthash in the livemap\n")
					// Get the corresponding liveMap hash
					//If there is a diff then log it!
					if y.ID != msg.ID {
						fmt.Printf("ID diff detected. Expected id: %d, got id: %d\n", y.ID, msg.ID)
					} else {
						fmt.Printf("No diff detected\n")
					}
				}
			}
			if !foundSent {
				// Throw some error as the message is not found at all
				fmt.Printf("id: %d, result: %s", msg.ID, msg.Result)
				fmt.Printf("ERROR: SENTMESSAGE HASH NOT FOUND\n")
			}
		}
	} else {
		fmt.Printf("No changes need to be made as no ID is provided with this message\n")
	}

	// When a request has been sent then add the responses to that request
	// Alter the ID of the response first though to match the sent
	//lastRequest.responses = append(lastRequest.responses, livemessage)

}

func hashString(message string) string {
	h := md5.New()
	h.Write([]byte(message))
	sha1Hash := hex.EncodeToString(h.Sum(nil))

	return sha1Hash
}
