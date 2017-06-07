// Copyright (c) 2017 Tigera, Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package syncproto

import (
	"encoding/gob"
	"time"

	log "github.com/Sirupsen/logrus"

	"reflect"

	"fmt"

	"github.com/projectcalico/libcalico-go/lib/backend/api"
	"github.com/projectcalico/libcalico-go/lib/backend/model"
)

const DefaultPort = 5473

type Envelope struct {
	Message interface{}
}

type MsgClientHello struct {
	Hostname string
	Info     string
	Version  string
}
type MsgServerHello struct {
	Version string
}
type MsgSyncStatus struct {
	SyncStatus api.SyncStatus
}
type MsgPing struct {
	Timestamp time.Time
}
type MsgPong struct {
	PingTimestamp time.Time
	PongTimestamp time.Time
}
type MsgKVs struct {
	KVs []SerializedUpdate
}

func init() {
	// We need to use RegisterName here to force the name to be equal, even if this package gets vendored since the
	// default name would include the vendor directory.
	gob.RegisterName("github.com/projectcalico/typha/pkg/syncproto.MsgClientHello", MsgClientHello{})
	gob.RegisterName("github.com/projectcalico/typha/pkg/syncproto.MsgServerHello", MsgServerHello{})
	gob.RegisterName("github.com/projectcalico/typha/pkg/syncproto.MsgSyncStatus", MsgSyncStatus{})
	gob.RegisterName("github.com/projectcalico/typha/pkg/syncproto.MsgPing", MsgPing{})
	gob.RegisterName("github.com/projectcalico/typha/pkg/syncproto.MsgPong", MsgPong{})
	gob.RegisterName("github.com/projectcalico/typha/pkg/syncproto.MsgKVs", MsgKVs{})
}

func SerializeUpdate(u api.Update) (kv SerializedUpdate, err error) {
	kv.Key, err = model.KeyToDefaultPath(u.Key)
	if err != nil {
		log.WithError(err).WithField("kv", u).Error(
			"Bug: failed to serialize key that was generated by Syncer.")
		return
	}
	kv.Value, err = model.SerializeValue(&u.KVPair)
	if err != nil {
		log.WithError(err).WithField("kv", u).Error(
			"Bug: failed to serialize value, converting to nil value.")
		var nilKV model.KVPair
		nilKV.Key = u.Key
		kv.Value, err = model.SerializeValue(&nilKV)
		if err != nil {
			log.WithError(err).WithField("kv", u.Key).Error(
				"Bug: Failed to serialize nil value for key.")
			return
		}
	}

	kv.TTL = u.TTL
	kv.Revision = u.Revision // This relies on the revision being a basic type.
	kv.UpdateType = u.UpdateType
	return
}

type SerializedUpdate struct {
	Key        string
	Value      []byte
	Revision   interface{}
	TTL        time.Duration
	UpdateType api.UpdateType
}

func (s SerializedUpdate) ToUpdate() api.Update {
	// Parse the key.
	parsedKey := model.KeyFromDefaultPath(s.Key)
	parsedValue, err := model.ParseValue(parsedKey, s.Value)
	if err != nil {
		log.WithField("rawValue", string(s.Value)).Error(
			"Failed to parse value.")
	}
	return api.Update{
		KVPair: model.KVPair{
			Key:      parsedKey,
			Value:    parsedValue,
			Revision: s.Revision,
			TTL:      s.TTL,
		},
		UpdateType: s.UpdateType,
	}
}

// WouldBeNoOp returns true if this update would be a no-op given that previous has already been sent.
func (s SerializedUpdate) WouldBeNoOp(previous SerializedUpdate) bool {
	// We don't care if the revision has changed so nil it out.  Note: we're using the fact that this is a
	// value type so these changes won't be propagated to the caller!
	s.Revision = nil
	previous.Revision = nil

	if previous.UpdateType == api.UpdateTypeKVNew {
		// If the old update was a create, convert it to an update before the comparison since it's OK to
		// squash an update to a new key if the value hasn't changed.
		previous.UpdateType = api.UpdateTypeKVUpdated
	}

	// TODO Typha Add UT to make sure that the serialization is always the same (current JSON impl does make that guarantee)
	return reflect.DeepEqual(s, previous)
}

func (s SerializedUpdate) String() string {
	return fmt.Sprintf("SerializedUpdate<Key:%s, Value:%s, Revision:%v, TTL:%v, UpdateType:%v>",
		s.Key, string(s.Value), s.Revision, s.TTL, s.UpdateType)
}
