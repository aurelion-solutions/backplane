// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package ingest_mq is the AMQP consumer that bridges connectors to
// inventory_ingest.
//
// Connectors publish raw records ONE PER MESSAGE into the
// aurelion.ingest topic exchange. Routing key = dataset_type;
// headers carry source and correlation_id. The body is whatever the
// connector wants the lake to store — at minimum it must contain
// external_id.
//
// This consumer buffers incoming messages keyed by
// (source, dataset_type, correlation_id), flushes a bucket when it
// reaches FlushSize records OR FlushInterval has elapsed since the
// bucket's first message, then calls inventory_ingest.Service.Process
// once on the entire window. After Process returns, every AMQP
// delivery in the window is acked.
//
// This package owns no business logic — it is a transport. The
// ingest engine itself is unaware MQ even exists.
package ingest_mq
