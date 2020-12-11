# MicroMDM_Bolt2MySQL - a tool to migrate MicroMDM bolt database to MySQL

MicroMDM_Bolt2MySQL is a tool to migrate Mobile Device Management server Bolt database to MySQL.

This tool partly resuse MicroMDM sources with MySQL support to extract data from Bolt Database and save to MySQL.

# Run MicroMDM_Bolt2MySQL

This tool can run using docker-compose command.

First copy your micromdm.db file to bolt_database directory

Start your MySQL server.
(Note, If you have no MySQL server you can clone https://github.com/SixK/micromdm 
and follow "Test MicroMDM with Mysql" steps)

Modify the following line in micromdm_bolt2mysql.go to let it connect to your MySQL server  :
>mydb, err = sqlx.Connect("mysql", "micromdm:micromdm@(localhost:3306)/micromdm")

Run docker-compose
>docker-compose up

# Notes
This tool is actually in Beta test and not fully tested.  
Use it at your own risk !!!  

Don't forget to copy your certificates from your MicroMDM server to your new MicroMDM server with MySQL Support

All elements from BoltDB are not yet extracted, but extracted keys should be sufficient to let MicroMDM with MySQL support work for most use cases.

If not running in docker container with docker-compose, sources from https://github.com/SixK/micromdm will be necessary.

https://github.com/SixK/micromdm is a modified fork from https://github.com/Lepidopteron/micromdm  
https://github.com/Lepidopteron/micromdm is a fork that add MySQL support to official MicroMDM projet https://github.com/micromdm/micromdm

If you have the following error then you probably not copied your micromdm.db file to bolt_database directory :
>micromdm_bolt2mysql_1  | 2020/12/11 16:37:50 Bucket "mdm.Devices" not found!
micromdm_boltdb2mysql_micromdm_bolt2mysql_1 exited with code 1

