module github.com/devian2011/msgchute

go 1.25.10

require (
	github.com/DATA-DOG/go-sqlmock v1.5.2
	github.com/Masterminds/squirrel v1.5.4
	github.com/ValerySidorin/shclog v0.0.1
	github.com/amacneil/dbmate/v2 v2.34.1
	github.com/bytedance/sonic v1.15.2
	github.com/devian2011/msgchute/pkg/shared/auth v0.0.0-20260810114130-cd023e52c314
	github.com/devian2011/msgchute/pkg/shared/provider v0.0.0-20260810114130-cd023e52c314
	github.com/devian2011/retrier v1.2.0
	github.com/flosch/pongo2/v7 v7.0.0-alpha.1
	github.com/go-chi/chi/v5 v5.3.1
	github.com/go-chi/cors v1.2.2
	github.com/go-chi/httplog/v3 v3.4.0
	github.com/go-playground/form v3.1.4+incompatible
	github.com/google/uuid v1.6.0
	github.com/gookit/config/v2 v2.2.8
	github.com/hashicorp/go-plugin v1.8.0
	github.com/jackc/pgx/v5 v5.10.0
	github.com/jmoiron/sqlx v1.4.0
	github.com/joho/godotenv v1.5.1
	github.com/stretchr/testify v1.11.1
	github.com/swaggo/http-swagger/v2 v2.0.2
	github.com/swaggo/swag v1.16.6
	golang.org/x/sync v0.22.0
)

require (
	dario.cat/mergo v1.0.2 // indirect
	github.com/KyleBanks/depth v1.2.1 // indirect
	github.com/bytedance/gopkg v0.1.3 // indirect
	github.com/bytedance/sonic/loader v0.5.1 // indirect
	github.com/cloudwego/base64x v0.1.6 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/fatih/color v1.17.0 // indirect
	github.com/go-openapi/jsonpointer v1.0.0 // indirect
	github.com/go-openapi/jsonreference v1.0.0 // indirect
	github.com/go-openapi/spec v0.22.9 // indirect
	github.com/go-openapi/swag/conv v0.28.0 // indirect
	github.com/go-openapi/swag/jsonutils v0.28.0 // indirect
	github.com/go-openapi/swag/loading v0.28.0 // indirect
	github.com/go-openapi/swag/pools v0.28.0 // indirect
	github.com/go-openapi/swag/stringutils v0.28.0 // indirect
	github.com/go-openapi/swag/typeutils v0.28.0 // indirect
	github.com/go-openapi/swag/yamlutils v0.28.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/goccy/go-yaml v1.19.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/gookit/goutil v0.7.5 // indirect
	github.com/hashicorp/go-hclog v1.6.3 // indirect
	github.com/hashicorp/yamux v0.1.2 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/lann/builder v0.0.0-20180802200727-47ae307949d0 // indirect
	github.com/lann/ps v0.0.0-20150810152359-62de8c46ede0 // indirect
	github.com/lib/pq v1.12.3 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/oklog/run v1.1.0 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/swaggo/files/v2 v2.0.2 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/arch v0.0.0-20210923205945-b76863e36670 // indirect
	golang.org/x/mod v0.38.0 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/term v0.45.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	golang.org/x/tools v0.48.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260427160629-7cedc36a6bc4 // indirect
	google.golang.org/grpc v1.81.1 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/go-playground/assert.v1 v1.2.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/devian2011/msgchute/pkg/helper => ./../pkg/

replace github.com/devian2011/retrier => github.com/notamfod/retrier v1.2.1-0.20260910064435-72ffea120e6d
