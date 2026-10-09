using System;
using System.Net.Http;
using System.Text.Json;
using System.Threading;
using System.Threading.Tasks;
#if NET7_0_OR_GREATER
using System.Diagnostics.CodeAnalysis;
#endif


namespace Misirka.HttpClient;

public record MisirkaHttpClientOptions(string Url);

// TODO: this is not done at all, both at the server and client
#if NET7_0_OR_GREATER
[method: SetsRequiredMembers]
#endif
public class MisirkaHttpClient(MisirkaHttpClientOptions options) : Misirka.MisirkaClient
{
    private readonly MisirkaHttpClientOptions _options = options;
    private readonly System.Net.Http.HttpClient _httpClient = new() { BaseAddress = new Uri(options.Url) };

    public override async Task ConnectAsync(CancellationToken token = default) => await OnAlive(EventArgs.Empty);

    public override Task<int> SubscribeAsync<TR>(string topic, AsyncEventHandler<TR> handler) =>
        throw new NotImplementedException();

    public override Task UnsubscribeAsync(int id, string topic) => throw new NotImplementedException();

    public override Task PingAsync(string? val = null) => throw new NotImplementedException();

    public override async Task<T> GetAsync<T>(string topic, Parser<T>? parser = null)
    {
        try
        {
            await using var httpResponse = await _httpClient.GetStreamAsync(topic);

            var response = await JsonSerializer.DeserializeAsync<T>(httpResponse,
                new JsonSerializerOptions
                {
                    RespectRequiredConstructorParameters = true
                });
            return response is not null ? response : throw new Exception();
        }
        catch (JsonException e)
        {
            try
            {
                // var error = await JsonSerializer.DeserializeAsync<object>(stream);
                // throw new Exception(error?.ToString(), e);
            }
            finally
            {
                throw e;
            }
        }
    }

    public override Task<(int, TR)> CallAsync<TP, TR>(string method, TP param, Parser<TR>? parser = null)
    {
        throw new NotImplementedException();
    }

    public override Task<int> CommandAsync<TP>(string method, TP param)
    {
        throw new NotImplementedException();
    }

    public override async Task DisconnectAsync(CancellationToken token = default)
    {
        await OnDead(EventArgs.Empty);
    }

    public override async ValueTask DisposeAsync()
    {
        _httpClient.Dispose();
        await OnDead(EventArgs.Empty);
    }
}