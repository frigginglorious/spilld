package spillbox

const createSQL = `
-- SQL schema for a spilldb single user mailbox, a.k.a. a spillbox.
--
-- Contains the metadata for a user email archive,
-- including any active draft messages.

-- For IMAP XAPPLEPUSHSERVICE.
CREATE TABLE IF NOT EXISTS ApplePushDevices (
	Mailbox          TEXT NOT NULL,
	AppleAccountID   TEXT NOT NULL,
	AppleDeviceToken TEXT NOT NULL
);

-- Contacts is a list of contacts.
-- It is generated from incoming email and curated by users.
-- ContactID == 1 is always the user of this account.
CREATE TABLE IF NOT EXISTS Contacts (
	ContactID   INTEGER PRIMARY KEY,
	Hidden      BOOLEAN, -- removed by user but still represents stored email
	Robot       BOOLEAN  -- not a human
);

-- Note: AddressID is used in messages under the assumption that Name/Address
-- do not change. For contact edits, create a new AddressID.
CREATE TABLE IF NOT EXISTS Addresses (
	AddressID   INTEGER PRIMARY KEY,
	ContactID   INTEGER,
	Name        TEXT,             -- display name component of address
	Address     TEXT NOT NULL,    -- address: user@domain
	DefaultAddr BOOLEAN NOT NULL, -- default address for this contact
	Visible     BOOLEAN NOT NULL, -- user has sent or mentioned this address

	FOREIGN KEY(ContactID) REFERENCES Contacts(ContactID)
);

-- Tie the mod-sequence used by CONDSTORE to the mailbox name.
--
-- MailboxID is not visible to IMAP, so reusing a deleted
-- mailbox's name will stomp on its old values.
-- In theory we handle this by incrementing UIDValidity, as
-- message uniqueness in IMAP is determined by:
--	(mailbox name, UIDVALIDITY, UID)
-- but that is not explicitly mentioned in RFC 7162 for
-- mod-sequences, so we play it safe and always increment
-- the value for a given mailbox name.
CREATE TABLE IF NOT EXISTS MailboxSequencing (
	Name            TEXT PRIMARY KEY,
	NextModSequence INTEGER NOT NULL  -- uint32, IMAP RFC 7162 CONDSTORE
);

CREATE TABLE IF NOT EXISTS Mailboxes (
	MailboxID       INTEGER PRIMARY KEY,
	NextUID         INTEGER NOT NULL, -- uint32, used by IMAP
	UIDValidity     INTEGER NOT NULL, -- incremented on rename or create with old name
	Attrs           INTEGER, -- imapserver.ListAttrFlag
	Name            TEXT,
	DeletedName     TEXT,    -- Old label name before deletion
	Subscribed      BOOLEAN,

	UNIQUE(Name)
);

CREATE INDEX IF NOT EXISTS MailboxesName ON Mailboxes (Name);

CREATE TABLE IF NOT EXISTS Labels (
	LabelID     INTEGER PRIMARY KEY,
	Label       TEXT,    -- NULL means the label is deleted

	UNIQUE(Label)
);

CREATE INDEX IF NOT EXISTS LabelsLabel ON Labels (Label);

CREATE TABLE IF NOT EXISTS Convos (
	ConvoID      INTEGER PRIMARY KEY,
	ConvoSummary TEXT     -- JSON encoding of mdb.ConvoSummary
);

CREATE TABLE IF NOT EXISTS ConvoContacts (
	ConvoID      INTEGER,
	ContactID    INTEGER,

	PRIMARY KEY(ConvoID, ContactID),
	FOREIGN KEY(ConvoID)   REFERENCES Convos(ConvoID),
	FOREIGN KEY(ContactID) REFERENCES Contacts(ContactID)
);

CREATE TABLE IF NOT EXISTS ConvoLabels (
       LabelID  INTEGER,
       ConvoID INTEGER,

       PRIMARY KEY(LabelID, ConvoID),
       FOREIGN KEY(LabelID) REFERENCES Labels(LabelID),
       FOREIGN KEY(ConvoID) REFERENCES Convos(ConvoID)
);

CREATE TABLE IF NOT EXISTS Msgs (
	MsgID         INTEGER PRIMARY KEY,
	StagingID     INTEGER, -- server staging ID, NULL for drafts
	ModSequence   INTEGER,
	Seed          INTEGER,
	RawHash       TEXT, -- sha256 of original input, NULL for drafts
	ConvoID       INTEGER,
	State         INTEGER, -- mdb.MsgState enum
	ParseError    TEXT,

	MailboxID  INTEGER,
	UID        INTEGER, -- uint32, used by IMAP, only filled out by server
	Flags      STRING,  -- JSON '{"flag": 1}' of IMAP flags
	-- TODO: are Flags a replacement for labels?

	EncodedSize INTEGER,

	-- Date is created by the server with time.Now().Unix(), that is,
	-- seconds since epoch.
	-- For drafts, it is the last time the message was edited.
	Date INTEGER NOT NULL,

	Expunged INTEGER, -- time message was expunged (time.Now().Unix())

	HdrsBlobID INTEGER,

	HasUnsubscribe INTEGER, -- HTML contains "<a>.*[Uu]nsubscribe</a>""

	UNIQUE (StagingID), -- may be NULL
	FOREIGN KEY(ConvoID) REFERENCES Convos(ConvoID),
	FOREIGN KEY(MailboxID) REFERENCES Mailboxes(MailboxID)
);

CREATE TABLE IF NOT EXISTS MsgAddresses (
	MsgID     INTEGER NOT NULL,
	AddressID INTEGER NOT NULL,
	Role      INTEGER NOT NULL, -- mdb.ContactRole (From:, To:, CC:, BCC:, etc)

	PRIMARY KEY(MsgID, AddressID, Role),
	FOREIGN KEY(MsgID) REFERENCES Msgs(MsgID),
	FOREIGN KEY(AddressID) REFERENCES Addresses(AddressID)
);

-- MsgParts contains the cleaved multipart MIME components of messages.
--
-- The parts are "flattened", so the MIME tree, if desired, needs to be
-- recreated using the msgbuilder package.
CREATE TABLE IF NOT EXISTS MsgParts (
	MsgID          INTEGER NOT NULL,
	PartNum        INTEGER NOT NULL,
	Name           TEXT NOT NULL,
	IsBody         BOOLEAN NOT NULL, -- text or html body of the email
	IsAttachment   BOOLEAN NOT NULL,
	IsCompressed   BOOLEAN, -- content is gzip compressed
	CompressedSize INTEGER,
	ContentType    TEXT,
	ContentID      TEXT,    -- mime header Content-ID
	BlobID         INTEGER, -- Blobs table key in the blobs database

	ContentTransferEncoding TEXT,
	ContentTransferSize     INTEGER,
	ContentTransferLines    INTEGER,

	PRIMARY KEY(MsgID, PartNum),
	FOREIGN KEY(MsgID) REFERENCES Msgs(MsgID)
);

-- TODO remove
INSERT OR IGNORE INTO Contacts (ContactID, Hidden, Robot) VALUES (1, FALSE, FALSE);
INSERT OR IGNORE INTO Labels (LabelID, Label) VALUES (1, 'Personal Mail');
INSERT OR IGNORE INTO Labels (LabelID, Label) VALUES (2, 'Subscriptions');
INSERT OR IGNORE INTO Labels (LabelID, Label) VALUES (3, 'Spam and Trash');

CREATE TRIGGER IF NOT EXISTS MailboxRenameUIDValidity
AFTER UPDATE OF Name ON Mailboxes
FOR EACH ROW
BEGIN
	UPDATE Mailboxes
		SET UIDValidity = (SELECT max(UIDValidity) FROM Mailboxes) + 1
		WHERE MailboxID = new.MailboxID;
END;

CREATE TABLE IF NOT EXISTS blobs.Blobs (
	BlobID  INTEGER PRIMARY KEY,
	SHA256  TEXT,    -- hash of the exact bytes stored in Content
	Deleted INTEGER, -- tombstone, unix seconds at blob garbage collection
	Content BLOB
);
`
