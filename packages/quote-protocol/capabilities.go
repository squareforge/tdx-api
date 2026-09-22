package quoteprotocol

type FunctionSupport string

const (
	FunctionSupportRaw      FunctionSupport = "raw"
	FunctionSupportTyped    FunctionSupport = "typed"
	FunctionSupportProtocol FunctionSupport = "protocol"
)

type FunctionCapability struct {
	FunctionDefinition
	Support FunctionSupport `json:"support"`
}

func FunctionCapabilities() []FunctionCapability {
	defs := Functions()
	out := make([]FunctionCapability, 0, len(defs))
	for _, def := range defs {
		out = append(out, FunctionCapability{
			FunctionDefinition: def,
			Support:            functionSupport(def.Value),
		})
	}
	return out
}

func functionSupport(value uint16) FunctionSupport {
	switch value {
	case RT_LOGIN, RT_KEEPACTIVE, RT_SERVERTIME, RT_STOCKINFOLIST, RT_REALTIME_EXT_INT64:
		return FunctionSupportTyped
	case RT_ZIPDATA:
		return FunctionSupportProtocol
	default:
		return FunctionSupportRaw
	}
}
