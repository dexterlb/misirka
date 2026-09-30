using System;
using System.Collections.Concurrent;
using System.Collections.Generic;
using System.Linq;
using System.Net.WebSockets;
using System.Text;
using System.Text.Json;
using System.Text.Json.Nodes;
using System.Threading;
using System.Threading.Tasks;
using Microsoft.Extensions.Logging;
using Misirka;

#if NET7_0_OR_GREATER
using System.Diagnostics.CodeAnalysis;
#endif

namespace Misirka.WsClient;

// TODO: make reconnect work correctly
public record MisirkaWsClientOptions(
    string Url,
    double? GetTimeout = 10,
    double? CallTimeout = 10,
    double? ReconnectPeriod = 10);

public class MisirkaWsClient : MisirkaClient
{
    /// <summary>
    /// Create a possibly concurrent dictionary, based on whether misirka multithreading is enabled or not,
    /// as configured in <seealso cref="MisirkaWsClient(MisirkaWsClientOptions, ILogger{MisirkaClient}?, bool)"/>
    /// </summary>
    /// <typeparam name="TKey">The type of the keys in the dictionary.</typeparam>
    /// <typeparam name="TValue">The type of the values in the dictionary.</typeparam>
    private IDictionary<TKey, TValue> CreateDictionary<TKey, TValue>()
    {
        return _useThreading ? new ConcurrentDictionary<TKey, TValue>() : new Dictionary<TKey, TValue>();
    }


#if NET7_0_OR_GREATER
    [SetsRequiredMembers]
#endif
#pragma warning disable MisirkaThreading1
    public MisirkaWsClient(MisirkaWsClientOptions options, ILogger<MisirkaClient>? logger = null)
        : this(options, logger, false)
    {
    }
#pragma warning restore MisirkaThreading1

#if NET7_0_OR_GREATER
    [SetsRequiredMembers]
#endif
#if NET8_0_OR_GREATER
    [Experimental("MisirkaThreading1")]
#endif
    public MisirkaWsClient(MisirkaWsClientOptions options, ILogger<MisirkaClient>? logger = null,
        bool useThreading = false)
    {
        _logger = logger;
        _useThreading = useThreading;
        _options = options;

        _subscriptions = CreateDictionary<string, IDictionary<int, AsyncEventHandler<byte[]>>>();
        _responseData = CreateDictionary<int, byte[]>();
        _responseWait = CreateDictionary<int, SemaphoreSlim>();

        MessageReceived = async (sender, args) =>
        {
            var response = JsonNode.Parse(args.Span) ?? throw new ArgumentNullException(nameof(args));
            var id = response["id"]?.GetValue<int>();
            var topic = response["topic"]?.GetValue<string>();

            var copied = new byte[args.Length];
            args.CopyTo(copied);

            if (!id.HasValue)
            {
                if (topic is null) throw new ArgumentException("Malformed response without neither id nor topic");

                if (!_subscriptions.TryGetValue(topic, out var subscription))
                    throw new ArgumentOutOfRangeException(nameof(args),
                        "Received a message for an un-subscribed topic");

                _logger?.LogIf(LogLevel.Trace, "Invoking subscription listeners for topic {topic}", topic);

                await Task.WhenAll(subscription.Select(x => x.Value.Invoke(sender, copied)));

                return;
            }

            if (!_responseWait.TryGetValue(id.Value, out var semaphore))
                throw new ArgumentException("Received response for non-existent request");

            // TODO: see if we can skip the copy here and move it to the actual response call, would skip one copy
            if (!_responseData.TryAdd(id.Value, copied))
                throw new ArgumentException($"Already received response for id {id}");

            _logger?.LogIf(LogLevel.Trace, "Received data for request {id}", id.Value);
            semaphore.Release();
        };
    }

    /// <summary>
    /// Connect to the configured websocket URL (<see cref="MisirkaWsClientOptions.Url"/>) and setup listeners 
    /// </summary>
    /// <remarks>This <see cref="Task"/> will resolve when the websocket dies</remarks>
    public override async Task ConnectAsync(CancellationToken token = default)
    {
        _logger.LogIf(LogLevel.Debug, "Connecting to websocket {url}", _options.Url);

        // TODO: detect when websocket is dead
        await _webSocket.ConnectAsync(new Uri(_options.Url), token);

        _logger.LogIf(LogLevel.Information, "Connected to websocket at {url}", _options.Url);

        // TODO: fix this
        if (_useThreading)
        {
            _logger.LogIf(LogLevel.Warning, "Threading support is very experimental");
            var threadStart = new ThreadStart(() => { _ = ReceiveLoop(token); });
            var thread = new Thread(threadStart);
            thread.Start();

            await OnAlive(EventArgs.Empty);

            await Task.Delay(Timeout.Infinite, token);
        }
        else
        {
            var receiveLoop = ReceiveLoop(token);
            await OnAlive(EventArgs.Empty);
            await receiveLoop;
        }
    }

    private readonly ClientWebSocket _webSocket = new();

    private event AsyncEventHandler<ReadOnlyMemory<byte>> MessageReceived;

    private readonly IDictionary<string, IDictionary<int, AsyncEventHandler<byte[]>>> _subscriptions;
    private readonly IDictionary<int, byte[]> _responseData;
    private readonly IDictionary<int, SemaphoreSlim> _responseWait;

    private readonly SemaphoreSlim _sendLock = new(1, 1);
    private readonly MisirkaWsClientOptions _options;

    private readonly Synchronizer<int> _idSource = new(0, x => x + 1);

    private readonly ILogger<MisirkaClient>? _logger;
    private readonly bool _useThreading;

    private async Task Send(ReadOnlyMemory<byte> data, CancellationToken token)
    {
        await _sendLock.WaitAsync(token);
        _logger?.LogIf(LogLevel.Trace, "Sending message to websocket: {message}", Encoding.UTF8.GetString(data.Span));
        await _webSocket.SendAsync(data, WebSocketMessageType.Text, true, token);
        _sendLock.Release();
        _logger?.LogIf(LogLevel.Trace, "Websocket message sent");
    }

    public override async Task DisconnectAsync(CancellationToken token = default)
    {
        //TODO: unsubscribe?
        await _webSocket.CloseAsync(WebSocketCloseStatus.NormalClosure, string.Empty, token);
        await OnDead(EventArgs.Empty);
    }

    public override async Task UnsubscribeAsync(int id, string topic)
    {
        var token = Misirka.Helpers.SecondsToTimeout(_options.CallTimeout);
        var req = new Request<string[]>("2.0", "ms-unsubscribe", [topic], id);

        _logger.LogIf(LogLevel.Trace, "Unsubscribing from request id {id}, topic {topic}", id, topic);

        _subscriptions[topic].Remove(id, out _);

        await CallAsync(req, token, Schemas.Ok);

        _logger.LogIf(LogLevel.Debug, "Unsubscribed from topic {topic} (id {id})", topic, id);
    }

    public override async Task PingAsync(string? val = null)
    {
        var id = await _idSource.GetNextAsync();
        var req = new Request<string>("2.0", "ms-ping", val, id);
        var token = Misirka.Helpers.SecondsToTimeout(_options.GetTimeout);

        var result = await CallAsync<string, string>(req, token);

        if (result != (val ?? "pong")) throw new ArgumentException("Misirka didn't pong correctly");
    }

    public override async Task<T> GetAsync<T>(string topic, Parser<T>? parser = null) =>
        (await CallAsync("ms-get", topic, parser)).Item2;

    public override async Task<(int, TR)> CallAsync<TP, TR>(string method, TP param, Parser<TR>? parser = null)
    {
        var id = await _idSource.GetNextAsync();

        var req = new Request<TP>("2.0", method, param, id);
        var token = Misirka.Helpers.SecondsToTimeout(_options.GetTimeout);

        return (id, await CallAsync(req, token, parser));
    }

    public override async Task<int> CommandAsync<TP>(string method, TP param) =>
        (await CallAsync(method, param, Schemas.Ok)).Item1;

    public override async Task<int> SubscribeAsync<TR>(string topic, AsyncEventHandler<TR> handler)
    {
        var id = await _idSource.GetNextAsync();
        var req = new Request<string[]>("2.0", "ms-subscribe", [topic], id);

        var token = Misirka.Helpers.SecondsToTimeout(_options.CallTimeout);

        if (!_subscriptions.ContainsKey(topic))
            _subscriptions[topic] = CreateDictionary<int, AsyncEventHandler<byte[]>>();
        if (!_subscriptions[topic].ContainsKey(id))
            _subscriptions[topic][id] = async (s, e) =>
                await handler(s, JsonSerializer.Deserialize<Message<TR>>(e)!.Contents!);
        else
            _subscriptions[topic][id] += async (s, e) =>
                await handler(s, JsonSerializer.Deserialize<Message<TR>>(e)!.Contents!);

        _logger?.LogIf(LogLevel.Trace, "Prepared subscription with id {id} for topic {topic}", id, topic);
        await CallAsync(req, token, Schemas.Ok);
        _logger?.LogIf(LogLevel.Trace, "Subscription acknowledged with id {id} for topic {topic}", id, topic);

        return id;
    }

    /// <summary>
    /// Preallocated web socket receive result
    ///
    /// This is an optimization for use in <see cref="ReceiveAll"/>
    /// </summary>
    private ValueWebSocketReceiveResult _receiveStatus;

    private async Task<int> ReceiveAll(Memory<byte> buf, CancellationToken token)
    {
        var i = 0;
        do
        {
            i += (_receiveStatus = await _webSocket.ReceiveAsync(buf[i..], token)).Count;
        } while (!_receiveStatus.EndOfMessage);

        return i;
    }


    private async Task ReceiveLoop(CancellationToken token)
    {
        var buf = new byte[10240];
        while (!token.IsCancellationRequested)
        {
            _logger?.LogIf(LogLevel.Trace, "Waiting for packet to receive");
            var bytes = await ReceiveAll(buf, token);
            var response = buf.AsMemory(0, bytes);
            _logger?.LogIf(LogLevel.Trace, "Received packet with length {bytes}: {request}", bytes,
                Encoding.UTF8.GetString(response.Span));

            _ = MessageReceived.Invoke(this, response)
                .ContinueWith(
                    task => { _logger?.LogIf(LogLevel.Error, task.Exception, "Subscriber threw an exception"); },
                    TaskContinuationOptions.OnlyOnFaulted);
        }
    }

    private async Task<TR> CallAsync<TP, TR>(Request<TP> req, CancellationToken token, Parser<TR>? parser = null)
    {
        _responseWait[req.Id] = new SemaphoreSlim(0, 1);
        _ = Send(req.ToJsonBytes(), token);

        _logger?.LogIf(LogLevel.Trace, "Waiting for response for call {id} for method {method}", req.Id, req.Method);
        await _responseWait[req.Id].WaitAsync(token);

        if (!_responseData.Remove(req.Id, out var response))
            throw new ArgumentException($"Received unsolicited response for request id {req.Id}");

        _logger?.LogIf(LogLevel.Debug, "Received response for request {id}", req.Id);

        // TODO: make this look better
        parser ??= x =>
        {
            var parsed = JsonSerializer.Deserialize<Response<TR>>(x,
                new JsonSerializerOptions { RespectRequiredConstructorParameters = true });
            return parsed is not null
                ? parsed.Result
                : throw new Exception("Cannot parse object, maybe specify a custom parser if you do binary?");
        };

        return parser(response);
    }

    public override async ValueTask DisposeAsync()
    {
        await _webSocket.CloseAsync(WebSocketCloseStatus.NormalClosure, string.Empty, CancellationToken.None);
        await OnDead(EventArgs.Empty);
        _webSocket.Dispose();

        GC.SuppressFinalize(this);
    }
}