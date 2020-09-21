package sql

import (
	"context"
	"encoding/json"
	"github.com/gobuffalo/pop/v5"
	"github.com/pkg/errors"
	"gopkg.in/square/go-jose.v2"

	"github.com/ory/hydra/jwk"
	"github.com/ory/hydra/x"
	"github.com/ory/x/sqlcon"
)

var _ jwk.Manager = &Persister{}

func (p *Persister) AddKey(ctx context.Context, set string, key *jose.JSONWebKey) error {
	out, err := json.Marshal(key)
	if err != nil {
		return errors.WithStack(err)
	}

	encrypted, err := p.r.KeyCipher().Encrypt(out)
	if err != nil {
		return errors.WithStack(err)
	}

	return sqlcon.HandleError(p.Connection(ctx).Create(&jwk.SQLData{
		Set:     set,
		KID:     key.KeyID,
		Version: 0,
		Key:     encrypted,
	}))
}

func (p *Persister) AddKeySet(ctx context.Context, set string, keys *jose.JSONWebKeySet) error {
	return p.transaction(ctx, func(ctx context.Context, c *pop.Connection) error {
		for _, key := range keys.Keys {
			out, err := json.Marshal(key)
			if err != nil {
				return errors.WithStack(err)
			}

			encrypted, err := p.r.KeyCipher().Encrypt(out)
			if err != nil {
				return err
			}

			if err := c.Create(&jwk.SQLData{
				Set:     set,
				KID:     key.KeyID,
				Version: 0,
				Key:     encrypted,
			}); err != nil {
				return sqlcon.HandleError(err)
			}
		}
		return nil
	})
}

func (p *Persister) GetKey(ctx context.Context, set, kid string) (*jose.JSONWebKeySet, error) {
	var j jwk.SQLData
	if err := p.Connection(ctx).
		Where("sid = ? AND kid = ?", set, kid).
		Order("created_at DESC").
		First(&j); err != nil {
		return nil, sqlcon.HandleError(err)
	}

	key, err := p.r.KeyCipher().Decrypt(j.Key)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var c jose.JSONWebKey
	if err := json.Unmarshal(key, &c); err != nil {
		return nil, errors.WithStack(err)
	}

	return &jose.JSONWebKeySet{
		Keys: []jose.JSONWebKey{c},
	}, nil
}

func (p *Persister) GetKeySet(ctx context.Context, set string) (*jose.JSONWebKeySet, error) {
	var js []jwk.SQLData
	if err := p.Connection(ctx).
		Where("sid = ?", set).
		Order("created_at DESC").
		All(&js); err != nil {
		return nil, sqlcon.HandleError(err)
	}

	if len(js) == 0 {
		return nil, errors.Wrap(x.ErrNotFound, "")
	}

	keys := &jose.JSONWebKeySet{Keys: []jose.JSONWebKey{}}
	for _, d := range js {
		key, err := p.r.KeyCipher().Decrypt(d.Key)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		var c jose.JSONWebKey
		if err := json.Unmarshal(key, &c); err != nil {
			return nil, errors.WithStack(err)
		}
		keys.Keys = append(keys.Keys, c)
	}

	if len(keys.Keys) == 0 {
		return nil, errors.WithStack(x.ErrNotFound)
	}

	return keys, nil
}

func (p *Persister) DeleteKey(ctx context.Context, set, kid string) error {
	return sqlcon.HandleError(p.Connection(ctx).RawQuery("DELETE FROM hydra_jwk WHERE sid=? AND kid=?", set, kid).Exec())
}

func (p *Persister) DeleteKeySet(ctx context.Context, set string) error {
	return sqlcon.HandleError(p.Connection(ctx).RawQuery("DELETE FROM hydra_jwk WHERE sid=?", set).Exec())
}
