package gen

import "testing"

func TestGen_EmptyMatch(t *testing.T) {
	files := emitSource(t, map[string]string{"App.alder": `module App
session { }
error : Error -> Html Never
error err = <p>error</p>
notFound : Html Never
notFound = <p>not found</p>
`, "Home.alder": `module Home

page "/" { }

type Model = { }

load : Params -> Session -> Query Model
load params session = { Query.succeed {} }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg { }

impossible : Msg -> Html Msg
impossible msg = match msg { }

view : Model -> Session -> Html Msg
view model session = <h1>Homepage</h1>
`})
	vet(t, files)
}
