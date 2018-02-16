package database

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/osum4est/awesome-sms-server/model"
	"strings"
)

const (
	messageTableName = "message"

	messageColId       = "id"
	messageColDate     = "date"
	messageColProtocol = "protocol"
	messageColThreadId = "thread_id"
	messageColSender   = "sender"
	messageColBody     = "body"

	messageCreateTableSql = "CREATE TABLE IF NOT EXISTS " + messageTableName + " (" +
		messageColId + " integer NOT NULL," +
		messageColDate + " integer NOT NULL," +
		messageColProtocol + " integer NOT NULL," +
		messageColThreadId + " integer NOT NULL," +
		messageColSender + " text," + // Null if you sent
		messageColBody + " text," + // Message body can be null
		"UNIQUE(" + messageColId + "," + messageColProtocol + "));"

	// TODO: Something...
	messageGetNewMessagesSql = `
SELECT *
FROM (
       SELECT
         message.id,
         message.date,
         message.protocol,
         message.thread_id,
         message.sender,
         message.body,
         thread_participant.phone,
         NULL AS attachment_id,
         NULL AS mime,
         NULL AS data
       FROM message
         LEFT OUTER JOIN thread_participant
           ON message.thread_id = thread_participant.thread_id
       UNION

       SELECT
         message.id,
         message.date,
         message.protocol,
         message.thread_id,
         message.sender,
         message.body,
         NULL          AS phone,
         attachment.id AS attachment_id,
         attachment.mime,
         attachment.data
       FROM message
         LEFT OUTER JOIN attachment
           ON message.id = attachment.message_id
     ) AS cols
  NATURAL JOIN (SELECT DISTINCT message.id
                FROM message
                LIMIT 0, ?)
WHERE cols.date>?
ORDER BY cols.thread_id, cols.id;
`

	// Faster than multiple queries for each thread. Probably...
	messageGetThreadsSql = `
SELECT *
FROM (
       SELECT
         message.id,
         message.date,
         message.protocol,
         message.thread_id,
         message.sender,
         message.body,
         thread_participant.phone,
         NULL AS attachment_id,
         NULL AS mime,
         NULL AS data
       FROM message
         LEFT OUTER JOIN thread_participant
           ON message.thread_id = thread_participant.thread_id
       UNION

       SELECT
         message.id,
         message.date,
         message.protocol,
         message.thread_id,
         message.sender,
         message.body,
         NULL          AS phone,
         attachment.id AS attachment_id,
         attachment.mime,
         attachment.data
       FROM message
         LEFT OUTER JOIN attachment
           ON message.id = attachment.message_id
     ) AS cols
--   Used to limit number of threads. For now we want all of them:
--   NATURAL JOIN (SELECT DISTINCT message.thread_id
--                 FROM message
--                 LIMIT 0, 3)
WHERE cols.id IN (
  SELECT message.id
  FROM message
  WHERE message.thread_id = cols.thread_id
  LIMIT 0, ?
)
ORDER BY cols.thread_id, cols.id;
`
)

type messageTable struct {
	sqlDb *sql.DB
}

func (table *messageTable) createIfNotExists() {
	execOrThrow(table.sqlDb, messageCreateTableSql)
}

func (table *messageTable) Insert(messages ...*model.MessageJson) {
	stmt := "INSERT OR IGNORE INTO " + messageTableName + " VALUES"
	data := make([]interface{}, len(messages)*6) // Each message has 6 columns

	// Compile all messages into 1 query
	for i, message := range messages {
		// Get sender
		var sender string
		for _, address := range message.Addresses {
			if address.Type == model.MessageAddressTypeFrom {
				sender = address.Address
			}
		}

		// Add to stmt and data
		stmt += "(?,?,?,?,?,?),"
		data[i*6+0] = message.Id
		data[i*6+1] = message.Date
		data[i*6+2] = message.Protocol
		data[i*6+3] = message.ThreadId
		data[i*6+4] = sender
		data[i*6+5] = message.Body
	}
	stmt = strings.TrimRight(stmt, ",")

	// Insert messages into db
	_, err := table.sqlDb.Exec(stmt, data...)
	if err != nil {
		panic(err)
	}
}

func (table *messageTable) GetNewMessages(afterDate int64, amount int) *[]model.MessageJson {
	rows, err := table.sqlDb.Query(messageGetNewMessagesSql, amount, afterDate)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	return table.getMessagesFromRows(rows)
}

func (table *messageTable) GetThreads(amount int) *[]model.MessageJson {

	rows, err := table.sqlDb.Query(messageGetThreadsSql, amount)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	return table.getMessagesFromRows(rows)
}

func (table *messageTable) getMessagesFromRows(rows *sql.Rows) *[]model.MessageJson {
	messages := make([]model.MessageJson, 0)
	lastId := -1

	for rows.Next() {
		message := &model.MessageJson{}

		var (
			sender       string
			phone        sql.NullString
			attachmentId sql.NullInt64
			mime         sql.NullString
			data         []byte
		)

		err := rows.Scan(
			&message.Id,
			&message.Date,
			&message.Protocol,
			&message.ThreadId,
			&sender,
			&message.Body,
			&phone,
			&attachmentId,
			&mime,
			&data)
		if err != nil {
			panic(err)
		}

		if lastId != message.Id {
			messages = append(messages, *message)
		} else {
			// Get the already created message if this message has already been created
			message = &messages[len(messages)-1]
		}

		// Add phone/attachment data
		if phone.Valid { // Is a phone row
			var addressType byte
			if sender == phone.String {
				addressType = model.MessageAddressTypeFrom
			} else {
				addressType = model.MessageAddressTypeCC
			}

			message.Addresses = append(message.Addresses,
				model.MessageAddressJson{
					Address: phone.String,
					Type:    addressType})
		} else { // Is an attachment row
			message.Attachments = append(message.Attachments,
				model.MessageAttachmentJson{
					Id:   int(attachmentId.Int64),
					Mime: mime.String,
					Data: data})
		}

		lastId = message.Id
	}

	return &messages
}