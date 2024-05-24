// Copyright 2023 ecodeclub
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dao

import (
	"context"

	"github.com/olivere/elastic/v7"
)

type anyESDAO struct {
	client *elastic.Client
}

func NewAnyEsDAO(client *elastic.Client) AnyDAO {
	return &anyESDAO{
		client: client,
	}
}

func (a *anyESDAO) Input(ctx context.Context, index string, docID string, data string) error {
	_, err := a.client.Index().
		Index(index).
		Id(docID).
		BodyJson(data).Do(ctx)
	return err
}
