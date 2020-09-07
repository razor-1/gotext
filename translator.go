/*
 * Copyright (c) 2018 DeineAgentur UG https://www.deineagentur.com. All rights reserved.
 * Licensed under the MIT License. See LICENSE file in the project root for full license information.
 */

package gotext

import (
	"errors"
	"io/ioutil"
	"net/textproto"
	"os"
	"path/filepath"
)

// ParseFile tries to read the file by its provided path (f) and parse its content as a .po or .mo file.
func ParseFile(f string) (gt GettextFile, err error) {
	// Check if file exists
	info, err := os.Stat(f)
	if err != nil {
		return
	}

	// Check that isn't a directory
	if info.IsDir() {
		err = errors.New("cannot parse a directory")
		return
	}

	ext := filepath.Ext(f)
	if ext == ".po" {
		gt = NewPo()
	} else if ext == ".mo" {
		gt = NewMo()
	} else {
		err = errors.New("unknown file type")
		return
	}

	// Parse file content
	data, err := ioutil.ReadFile(f)
	if err != nil {
		return
	}

	gt.Parse(data)

	return gt, nil
}

type GettextFile interface {
	Parse(buf []byte)
	GetDomain() *Domain
	Get(str string, vars ...interface{}) string
	GetN(str, plural string, n int, vars ...interface{}) string
	GetC(str, ctx string, vars ...interface{}) string
	GetNC(str, plural string, n int, ctx string, vars ...interface{}) string
}

// TranslatorEncoding is used as intermediary storage to encode Translator objects to Gob.
type TranslatorEncoding struct {
	// Headers storage
	Headers textproto.MIMEHeader

	// Language header
	Language string

	// Plural-Forms header
	PluralForms string

	// Parsed Plural-Forms header values
	Nplurals int
	Plural   string

	// Storage
	Translations map[string]*Translation
	Contexts     map[string]map[string]*Translation
}

// GetTranslator is used to recover a Translator object after unmarshalling the TranslatorEncoding object.
// Internally uses a Po object as it should be switchable with Mo objects without problem.
// External Translator implementations should be able to serialize into a TranslatorEncoding object in order to
// deserialize into a Po-compatible object.
func (te *TranslatorEncoding) GetFile() GettextFile {
	po := new(Po)
	po.domain = NewDomain()
	po.domain.Headers = te.Headers
	po.domain.Language = te.Language
	po.domain.PluralForms = te.PluralForms
	po.domain.nplurals = te.Nplurals
	po.domain.plural = te.Plural
	po.domain.translations = te.Translations
	po.domain.contexts = te.Contexts

	return po
}
