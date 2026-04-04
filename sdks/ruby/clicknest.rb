# frozen_string_literal: true

require "json"
require "net/http"
require "uri"

# ClickNest Ruby SDK for server-side analytics.
module ClickNest
  class Client
    # @param api_key [String]
    # @param host [String]
    # @param flush_interval [Integer] seconds between flushes (default 10)
    # @param max_batch_size [Integer] max events before auto-flush (default 50)
    def initialize(api_key:, host:, flush_interval: 10, max_batch_size: 50)
      @api_key = api_key
      @host = host.chomp("/")
      @max_batch_size = max_batch_size
      @queue = []
      @mutex = Mutex.new

      @running = true
      @thread = Thread.new do
        while @running
          sleep flush_interval
          flush
        end
      end

      at_exit { shutdown }
    end

    # Track an event.
    # @param event [String] event name
    # @param distinct_id [String, nil]
    # @param properties [Hash, nil]
    def capture(event, distinct_id: nil, properties: {})
      item = {
        event: {
          event_type: "custom",
          url: event,
          url_path: event,
          timestamp: (Time.now.to_f * 1000).to_i,
          properties: (properties || {}).merge("$event_name" => event),
        },
        distinct_id: distinct_id || "",
      }

      @mutex.synchronize do
        @queue << item
        send_batch if @queue.size >= @max_batch_size
      end
    end

    # Link an anonymous ID to an identified user.
    # @param distinct_id [String]
    # @param previous_id [String, nil]
    def identify(distinct_id, previous_id: nil)
      return unless previous_id

      capture("$identify", distinct_id: distinct_id, properties: { "previous_id" => previous_id })
    end

    # Force flush all queued events.
    def flush
      @mutex.synchronize { send_batch }
    end

    # Flush and stop the background thread.
    def shutdown
      @running = false
      flush
    end

    private

    def send_batch
      return if @queue.empty?

      items = @queue.dup
      @queue.clear

      # Group by distinct_id.
      batches = items.group_by { |i| i[:distinct_id] }

      batches.each do |did, group|
        body = {
          events: group.map { |i| i[:event] },
          session_id: "server-#{Time.now.to_i}",
          distinct_id: did,
        }.to_json

        uri = URI("#{@host}/api/v1/events")
        req = Net::HTTP::Post.new(uri)
        req["Content-Type"] = "application/json"
        req["X-API-Key"] = @api_key
        req.body = body

        Net::HTTP.start(uri.hostname, uri.port, use_ssl: uri.scheme == "https", open_timeout: 5, read_timeout: 5) do |http|
          http.request(req)
        end
      rescue StandardError
        # Best effort — don't crash the app.
      end
    end
  end
end
