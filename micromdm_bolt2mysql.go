package main

import (
    "fmt"
    "flag"
    "log"
    "time"
    "context"
    "strings"
    "math/big"
    
    "database/sql"

    "github.com/micromdm/go4/env"

    "github.com/pkg/errors"
    "github.com/boltdb/bolt"

    _ "github.com/go-sql-driver/mysql"
    "github.com/jmoiron/sqlx"
    "github.com/micromdm/micromdm/platform/device"
    devicemysql "github.com/micromdm/micromdm/platform/device/mysql"
    devicebolt "github.com/micromdm/micromdm/platform/device/builtin"

    "github.com/micromdm/micromdm/platform/profile"
    profilemysql "github.com/micromdm/micromdm/platform/profile/mysql"

    apnsmysql "github.com/micromdm/micromdm/platform/apns/mysql"
    apnsbolt "github.com/micromdm/micromdm/platform/apns/builtin"

    "github.com/micromdm/micromdm/platform/config"
    configmysql "github.com/micromdm/micromdm/platform/config/mysql"

    "github.com/micromdm/micromdm/platform/queue"

    sq "gopkg.in/Masterminds/squirrel.v1"
)

/*
 * Mapping micromdm BoltDB / MySQL
 *
    Target                  Source
  | cursors            |
- | dep_auto_assign    | mdm.DEPAutoAssign
  | dep_tokens         | --> mdm.DEPToken bdd deptoken.db       id=3  -->  DEPToken
* | device_commands    | mdm.DeviceCommands                     + Not all commands exported
* | devices            | mdm.Devices                            +
* | profiles           | mdm.Profile                            +
* | push_info          | mdm.PushInfo                           +
- | remove_device      | mdm.RemoveDevice
* | scep_certificates  | scep_certificates                      +
* | server_config      | scep_certificates                      id=4 +  --> Push_Certificate + Private_Key
* | server_config      | mdm.ServerConfig                       id=1 +  --> CA Root
- | server_config      | mdm.DEPToken                           id=2 --> DEPkeyPair
* | uuid_cert_auth     | mdm.UDIDCertAuth                       +
*/


var (
    db *bolt.DB
    mydb *sqlx.DB
)

func init() {
    var err error
    // Open the my.db data file in the current directory.
    // It will be created if it doesn't exist.
    log.Println("in Init !")

    flMysqlUsername     := *flag.String("mysql-username", env.String("MICROMDM_MYSQL_USER", "micromdm"), "Username to login to Mysql")
    flMysqlPassword     := *flag.String("mysql-password", env.String("MICROMDM_MYSQL_PASSWORD", "micromdm"), "Password to login to Mysql")
    flMysqlDatabase     := *flag.String("mysql-database", env.String("MICROMDM_MYSQL_DATABASE", "micromdm"), "Name of the Mysql Database")
    flMysqlHost         := *flag.String("mysql-host", env.String("MICROMDM_MYSQL_HOST", "localhost"), "IP or URL to the Mysql Host")
    flMysqlPort         := *flag.String("mysql-port", env.String("MICROMDM_MYSQL_PORT", "3306"), "Port to use for Mysql connection")


    db, err = bolt.Open("/tmp/micromdm.db", 0600, &bolt.Options{Timeout: 1 * time.Second})
    if err != nil { log.Fatal(err) }

    // mydb, err = sqlx.Connect("mysql", "micromdm:micromdm@(localhost:3306)/micromdm")
    mydb, err = sqlx.Connect("mysql", 
                             fmt.Sprintf("%s:%s@(%s:%s)/%s", 
                                         flMysqlUsername, 
                                         flMysqlPassword, 
                                         flMysqlHost, 
                                         flMysqlPort, 
                                         flMysqlDatabase))
    if mydb == nil { log.Println("hey that's bad !!!") }

    if err != nil { log.Fatalln(err) }

    // _ = mydb
}

func printBucketInfo(bucketName []byte){
    key := []byte("nobody")
    value, length := queryDB(bucketName, key)
    fmt.Printf("---->%s [%d]\n", string(value), length)
    if string(value) == "" { fmt.Println("blanko") }
    fmt.Printf("init key [%s] => [%#v]\n", key, value)
}


func main() {
    defer db.Close()
    // defer mydb.Close()

    //////////////////////////////////////////////////////////////
    //// Devices
    //// devices            | mdm.Devices
    //// uuid_cert_auth     | mdm.UDIDCertAuth
    bucketName := []byte("mdm.Devices")
    printBucketInfo(bucketName)
    iterateDeviceDB(bucketName)

    bucketName = []byte("mdm.Profile")
    printBucketInfo(bucketName)
    iterateProfileDB(bucketName)

    bucketName = []byte("scep_certificates")
    printBucketInfo(bucketName)
    iterateCertificatesDB(bucketName)

    bucketName = []byte("mdm.ServerConfig")
    printBucketInfo(bucketName)
    iterateServerConfigDB(bucketName)

    bucketName = []byte("mdm.DeviceCommands")
    printBucketInfo(bucketName)
    iterateCommandsDB(bucketName)

}

// queryDB : retrieve data
func queryDB(bucketName, key []byte) (val []byte, length int) {
    err := db.View(func(tx *bolt.Tx) error {
        bkt := tx.Bucket(bucketName)
        if bkt == nil {
            return fmt.Errorf("Bucket %q not found!", bucketName)
        }
        val = bkt.Get(key)
        return nil
    })
    if err != nil { log.Fatal(err) }
    return val, len(string(val))
}

func command_columns() []string {
    return []string{
        "uuid",
        "device_udid",
        "payload",
        "created_at",
        "last_sent_at",
        "acknowledged_at",
        "times_sent",
        "last_status",
        "failure_message",
        "command_order",
    }
}

func SaveCommand(ctx context.Context, cmd queue.Command, deviceUDID string, order int) error {
    // Make sure we take the time offset into account for "zero" dates
    t := time.Now()
    _, offset := t.Zone()

    // Don't multiply by zero
    if (offset <= 0) {
        offset = 1
    }
    var min_timestamp_sec int64 = int64(offset) * 60 * 60 * 24

    if (cmd.CreatedAt.IsZero() || cmd.CreatedAt.Unix() < min_timestamp_sec) {
        cmd.CreatedAt = time.Unix(min_timestamp_sec, 0)
    }

    if (cmd.LastSentAt.IsZero() || cmd.LastSentAt.Unix() < min_timestamp_sec) {
        cmd.LastSentAt = time.Unix(min_timestamp_sec, 0)
    }

    if (cmd.Acknowledged.IsZero() || cmd.Acknowledged.Unix() < min_timestamp_sec) {
        cmd.Acknowledged = time.Unix(min_timestamp_sec, 0)
    }

    updateQuery, args_update, err := sq.StatementBuilder.
        PlaceholderFormat(sq.Question).
        Update("device_commands").
        Prefix("ON DUPLICATE KEY").
        Set("uuid", cmd.UUID).
        Set("device_udid", deviceUDID).
        Set("payload", cmd.Payload).
        Set("created_at", cmd.CreatedAt).
        Set("last_sent_at", cmd.LastSentAt).
        Set("acknowledged_at", cmd.Acknowledged).
        Set("times_sent", cmd.TimesSent).
        Set("last_status", cmd.LastStatus).
        Set("failure_message", cmd.FailureMessage).
        Set("command_order", order).
        ToSql()
    if err != nil {
        return errors.Wrap(err, "building update query for command save")
    }

    // MySql Convention
    // Replace "ON DUPLICATE KEY UPDATE TABLE_NAME SET" to "ON DUPLICATE KEY UPDATE"
    updateQuery = strings.Replace(updateQuery, "device_commands"+" SET ", "", -1)

    query, args, err := sq.StatementBuilder.
        PlaceholderFormat(sq.Question).
        Insert("device_commands").
        Columns(command_columns()...).
        Values(
            cmd.UUID,
            deviceUDID,
            cmd.Payload,
            cmd.CreatedAt,
            cmd.LastSentAt,
            cmd.Acknowledged,
            cmd.TimesSent,
            cmd.LastStatus,
            cmd.FailureMessage,
            order,
        ).
        Suffix(updateQuery).
        ToSql()

    var all_args = append(args, args_update...)

    if err != nil {
        return errors.Wrap(err, "building command save query")
    }

    // fmt.Println(query)

    _, err = mydb.ExecContext(ctx, query, all_args...)
    if err != nil { fmt.Println(err) }
    
    return errors.Wrap(err, "exec command save in mysql")
}


// iterateCommandsDB : iterate data
func iterateCommandsDB(bucketName []byte) {

    ctx := context.Background()

    if mydb == nil { log.Println("mydb is nil !!!") }

    err := db.View(func(tx *bolt.Tx) error {
        b := tx.Bucket(bucketName)

        c := b.Cursor()

        for k, v := c.First(); k != nil; k, v = c.Next() {
            cmdObj:=queue.DeviceCommand{}
            queue.UnmarshalDeviceCommand(v, &cmdObj)
            fmt.Printf("key=>[%s], value=[%s][%d %d %d %d]\n", k, cmdObj.DeviceUDID, len(cmdObj.Commands), len(cmdObj.Completed), len(cmdObj.Failed), len(cmdObj.NotNow) )

            var err error

            for i, _command := range cmdObj.Commands {
                err = SaveCommand(ctx, _command, cmdObj.DeviceUDID, i)
                if err != nil {
                    fmt.Println("Command in Error :", err)
                }
            }

        }
        return nil
    })
    if err != nil { log.Fatalf("failure : %s\n", err) }
}



// iterateServerConfigDB : iterate data
func iterateServerConfigDB(bucketName []byte) {
    var err2 error

    ctx := context.Background()

    if mydb == nil { log.Println("mydb is nil !!!") }

    myConfigDb ,err2 := configmysql.NewDB(mydb, nil)
    if err2 != nil { log.Fatal(err2) }

    err := db.View(func(tx *bolt.Tx) error {
        b := tx.Bucket(bucketName)

        c := b.Cursor()

        configObj:=config.ServerConfig{}

        var id int=1
        for k, v := c.First(); k != nil; k, v = c.Next() {
            config.UnmarshalServerConfig(v, &configObj)
            fmt.Printf("key=>[%s], value=[%s]\n", k, configObj.PushCertificate)

            if id == 1 {
                err := myConfigDb.SavePushCertificate(ctx, configObj.PushCertificate, configObj.PrivateKey)
                if err != nil { log.Fatal(err) }
            }else {
                fmt.Print("Will have to insert in DB\n")
            }

            id+=1
        }
        return nil
    })
    if err != nil { log.Fatalf("failure : %s\n", err) }
}



// iterateCertificatesDB : iterate data
func iterateCertificatesDB(bucketName []byte) {
    var ca_certificate []byte = nil
    var ca_key []byte = nil
    var serial big.Int

    if mydb == nil { log.Println("mydb is nil !!!") }

    err := db.View(func(tx *bolt.Tx) error {
        b := tx.Bucket(bucketName)

        c := b.Cursor()

        var addToMySQL bool
        var cert_name string
        for k, v := c.First(); k != nil; k, v = c.Next() {
            fmt.Printf("scep key=>[%s], value=[%s]\n", k, v)

            if string(k) == "ca_certificate" || string(k) == "ca_key" || string(k) == "serial" {
                fmt.Println("Special key found !!!")

                if string(k) == "ca_certificate" {
                    ca_certificate = v
                }

                if string(k) == "ca_key" {
                    ca_key = v
                }

                if string(k) == "serial" {
                    serial = *serial.SetBytes(v)
                }

                continue
            }


            // Verify if value already exists before inserting
            selectQuery, args_select, err := sq.StatementBuilder.
                PlaceholderFormat(sq.Question).
                Select("cert_name").
                From("scep_certificates").
                Where(sq.Eq{"cert_name": k}).
                ToSql()

            row := mydb.QueryRow(selectQuery, args_select[0])
            err = row.Scan(&cert_name)

            addToMySQL=false
            if err != nil {
                if err == sql.ErrNoRows {
                    print("value does not exists !!!")
                    addToMySQL=true
                }else{ fmt.Println("error :", err) }
            } else
            {
                fmt.Println("value already exists")
                addToMySQL=false
            }

            // fmt.Println(selectQuery)

            if addToMySQL {
                // No need to get serial, we have an auto_increment in place
                query, args, err := sq.StatementBuilder.
                    PlaceholderFormat(sq.Question).
                    Insert("scep_certificates").
                    Columns("cert_name", "scep_cert").
                    Values( k, v,).
                    ToSql()
                if err != nil { fmt.Println("error :", err) }

                // fmt.Println(query)

                ctx := context.Background()
                _, err = mydb.ExecContext(ctx, query, args...)
            }
        }
        return nil
    })

    if ca_certificate != nil && ca_key != nil && serial.Cmp(big.NewInt(0)) != 0 {
        fmt.Println("data : ", &serial, ca_certificate, ca_key)

	// mapping ca_certificate == ca_key and private_key == ca_certificate is probably wrong
	// but original micromdm_mysql has done this like that so ...
        updateQuery, args_update, err := sq.StatementBuilder.
                PlaceholderFormat(sq.Question).
                Update("server_config").
                Prefix("ON DUPLICATE KEY").
                Set("config_id", 4).
                Set("push_certificate", ca_key).
                Set("private_key", ca_certificate).
                ToSql()
        if err != nil { fmt.Println("error:", err) }

        // MySql Convention
        // Replace "ON DUPLICATE KEY UPDATE TABLE_NAME SET" to "ON DUPLICATE KEY UPDATE"
        updateQuery = strings.Replace(updateQuery, "server_config SET ", "", -1)

        query, args, err := sq.StatementBuilder.
            PlaceholderFormat(sq.Question).
            Insert("server_config").
            Columns("config_id", "push_certificate", "private_key").
            Values(
                4,
                ca_key,
                ca_certificate,
            ).
            Suffix(updateQuery).
            ToSql()
        if err != nil { fmt.Println("error:", err) }

        var all_args = append(args, args_update...)

        ctx := context.Background()
        _, err = mydb.ExecContext(ctx, query, all_args...)
        if err != nil { fmt.Println("error:", err) }

        /*
        // Probably not necessary with an Empty database
        // Note micromdm user should be granted to update INFORMATION_SCHEMA.TABLES
        query, args, err = sq.StatementBuilder.
            PlaceholderFormat(sq.Question).
            Update("INFORMATION_SCHEMA.TABLES").
            Set("AUTO_INCREMENT", serial.Uint64()).
            Where("TABLE_NAME = 'scep_certificates'").
            ToSql()

        // ctx := context.Background()
        _, err = mydb.ExecContext(ctx, query, args...)
        if err != nil { fmt.Println("error:", err) }
        */
    }

    if err != nil { log.Fatalf("failure : %s\n", err) }
}



// iterateDeviceDB : iterate data
func iterateProfileDB(bucketName []byte) {
    var err2 error

    ctx := context.Background()

    if mydb == nil { log.Println("mydb is nil !!!") }

    myProfileDb ,err2 := profilemysql.NewDB(mydb)
    if err2 != nil { log.Fatal(err2) }

    err := db.View(func(tx *bolt.Tx) error {
        b := tx.Bucket(bucketName)

        c := b.Cursor()

        profileObj:=profile.Profile{}

        for k, v := c.First(); k != nil; k, v = c.Next() {
            profile.UnmarshalProfile(v, &profileObj)
            fmt.Printf("key=>[%s], value=[%s]\n", k, profileObj.Mobileconfig)

            err := myProfileDb.Save(ctx, &profileObj)
            if err != nil { log.Fatal(err) }
        }
        return nil
    })
    if err != nil { log.Fatalf("failure : %s\n", err) }
}




// iterateDeviceDB : iterate data
func iterateDeviceDB(bucketName []byte) {
    var err2 error
    var err3 error

    ctx := context.Background()

    if mydb == nil { log.Println("mydb is nil !!!") }

    myDeviceDb ,err2 := devicemysql.NewDB(mydb)
    if err2 != nil { log.Fatal(err2) }

    myInfosDb, err3 := apnsmysql.NewDB(mydb, nil)
    if err3 != nil { log.Fatal(err3) }


    err := db.View(func(tx *bolt.Tx) error {
        b := tx.Bucket(bucketName)

        c := b.Cursor()

        deviceObj:=device.Device{}

        for k, v := c.First(); k != nil; k, v = c.Next() {
            device.UnmarshalDevice(v, &deviceObj)
            fmt.Printf("key=>[%s], value=[%s]\n", k, deviceObj.UUID)

            deviceDb, err:=devicebolt.NewDB(db)
            if err != nil { log.Fatal(err) }

            hash, err:=deviceDb.GetUDIDCertHash(ctx, []byte(deviceObj.UDID))
            if err != nil { log.Fatal(err) }

            fmt.Printf("udid: %s hash %s\n", deviceObj.UDID, hash)

            infosDb, err:= apnsbolt.NewDB(db, nil)
            pushInfoObj, err:=infosDb.PushInfo(ctx, deviceObj.UDID)
            if err != nil { log.Fatal(err) }


            myInfosDb.Save(ctx, pushInfoObj)
            myDeviceDb.SaveUDIDCertHash(ctx, []byte(deviceObj.UDID), hash)
            myDeviceDb.Save(ctx, &deviceObj)
        }
        return nil
    })
    if err != nil { log.Fatalf("failure : %s\n", err) }
}
