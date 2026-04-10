package main

import (
  "encoding/binary"
  "fmt"
  "os"
  "path/filepath"
  "bytes"
  "github.com/amazon-ion/ion-go/ion"
)

var sharedTable = ion.NewSharedSymbolTable("YJ_symbols", 10, func() []string {
  symbols := make([]string, 991)
  for sid := 10; sid <= 1000; sid++ {
    symbols[sid-10] = fmt.Sprintf("$%d", sid)
  }
  return symbols
}())

func main() {
  path := filepath.Join("..", "REFERENCE", "kfx_examples", "Martyr_5AFAFAA13FFE43ECBE78F0FF3761814C.kfx")
  data, err := os.ReadFile(path)
  if err != nil { panic(err) }
  cio := int(binary.LittleEndian.Uint32(data[10:14]))
  cil := int(binary.LittleEndian.Uint32(data[14:18]))
  info := data[cio:cio+cil]
  dec := ion.NewDecoder(ion.NewReaderCat(bytes.NewReader(info), ion.NewCatalog(sharedTable)))
  v, err := dec.Decode()
  if err != nil { panic(err) }
  m := normalize(v)
  dso := m["$415"].(int64)
  dsl := m["$416"].(int64)
  docSymbols := data[dso:dso+dsl]

  var buf bytes.Buffer
  w := ion.NewBinaryWriter(&buf)
  if err := w.WriteInt(0); err != nil { panic(err) }
  if err := w.Finish(); err != nil { panic(err) }
  stream := append(append([]byte{}, docSymbols...), buf.Bytes()[4:]...)
  r := ion.NewReaderCat(bytes.NewReader(stream), ion.NewCatalog(sharedTable))
  for r.Next() { break }
  if err := r.Err(); err != nil { panic(err) }
  table := r.SymbolTable()
  ids := []uint64{852,853,854,855,861,862,863,864,945,954,955,956,963,964,965,1000}
  for _, id := range ids {
    if sym, ok := table.FindByID(id); ok {
      fmt.Printf("%d -> %s\n", id, sym)
    } else {
      fmt.Printf("%d -> <missing>\n", id)
    }
  }
}

func normalize(v any) any {
  switch t := v.(type) {
  case *string:
    return *t
  case *ion.SymbolToken:
    if t.Text != nil { return *t.Text }
    return fmt.Sprintf("$%d", t.LocalSID)
  case ion.SymbolToken:
    if t.Text != nil { return *t.Text }
    return fmt.Sprintf("$%d", t.LocalSID)
  case map[string]any:
    out := map[string]any{}
    for k, vv := range t { out[k]=normalize(vv) }
    return out
  case []any:
    out := make([]any,len(t)); for i,vv := range t { out[i]=normalize(vv) }; return out
  case int:
    return int64(t)
  default:
    return t
  }
}
