package persistant

import (
  "errors"
  "log"
  "io/ioutil"
  "encoding/json"
)

func SaveState(f string, i interface{}) error {

  if f == "" { return errors.New("no file") }

  log.Printf("Saving %s", f)

  j, _ := json.MarshalIndent(i,"","  ")

  _ = ioutil.WriteFile(f, j, 0644)
  return nil
}

func LoadState(f string, i interface{}) error {

  if f == "" { return errors.New("no file") }

  j, err := ioutil.ReadFile(f)
  if err != nil {
    return err
  }
  _ = json.Unmarshal(j, i)

  return nil
}
/*
  type v struct {
    Name  string
    Id    string
    OnSim bool
  }

func main() {
  n :=  make(map[uuid.UUID]*v)

  LoadState("avatar.json", &n)
  log.Printf("n: %v", n)
  for k, v := range n {
    log.Printf("n[%s] = %v", k, v)
  }

}
*/
