using System.Text.Json.Serialization;

namespace Misirka.WsClient;

public record Request<T>(
    [property: JsonPropertyName("jsonrpc")]
    string Version,
    [property: JsonRequired, JsonPropertyName("method")]
    string Method,
    [property: JsonRequired, JsonPropertyName("params")]
    T? Params,
    [property: JsonRequired, JsonPropertyName("id")]
    int Id);

public record Response<T>(
    [property: JsonRequired, JsonPropertyName("result")]
    T Result,
    [property: JsonRequired, JsonPropertyName("id")]
    int Id
);

// TODO: this is not used, in favor of direct JsonNode parsing, in order to reduce memory thrashing
public record TopicMessage(
    [property: JsonPropertyName("topic")] string? Topic,
    [property: JsonPropertyName("id")] int? Id
);

public record Message<T>(
    [property: JsonRequired, JsonPropertyName("topic")]
    string Topic,
    [property: JsonRequired, JsonPropertyName("msg")]
    T Contents);

public record Error(
    [property: JsonRequired, JsonPropertyName("error")]
    string Message,
    [property: JsonRequired, JsonPropertyName("id")]
    int? Id);