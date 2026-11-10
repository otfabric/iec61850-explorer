<?xml version="1.0" encoding="UTF-8"?>
<SCL xmlns="http://www.iec.ch/61850/2003/SCL" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" version="2007" revision="B" release="4">
	<Header id="AA1_Northfield" version="3" revision="B" toolID="OT Fabric demo generator 1.0" nameStructure="IEDName">
		<Text>AA1 Northfield 110/20 kV substation: system configuration. Generated demo data, not a real installation.</Text>
		<History>
			<Hitem version="1" revision="A" when="2026-09-14T10:12:00Z" who="j.devries (substation engineering)" what="Initial system specification (SSD)" why="Project start"/>
			<Hitem version="2" revision="A" when="2026-10-02T14:40:00Z" who="j.devries (substation engineering)" what="IEDs added, data flow engineered" why="Detailed design"/>
			<Hitem version="3" revision="B" when="2026-11-03T08:55:00Z" who="m.okafor (commissioning)" what="GOOSE APPIDs and VLANs aligned with the network plan" why="FAT findings"/>
		</History>
	</Header>
	<Substation name="AA1" desc="Northfield 110/20 kV">
		<PowerTransformer name="T1" type="PTR" desc="110/20 kV 40 MVA YNd11">
			<LNode iedName="AA1E1Q01A1" ldInst="CTRL" lnClass="YPTR" lnInst="1"/>
			<LNode iedName="AA1E1Q01A1" ldInst="PROT" lnClass="PDIF" lnInst="1"/>
			<TransformerWinding name="W1" type="PTW" desc="110 kV winding">
				<Terminal name="T1" connectivityNode="AA1/E1/Q01/CN2" substationName="AA1" voltageLevelName="E1" bayName="Q01" cNodeName="CN2"/>
				<TapChanger name="LTC" type="LTC">
					<LNode iedName="AA1E1Q01A1" ldInst="CTRL" lnClass="YLTC" lnInst="1"/>
					<LNode iedName="AA1E1Q01A1" ldInst="CTRL" lnClass="ATCC" lnInst="1"/>
				</TapChanger>
			</TransformerWinding>
			<TransformerWinding name="W2" type="PTW" desc="20 kV winding">
				<Terminal name="T1" connectivityNode="AA1/D1/Q01/CN2" substationName="AA1" voltageLevelName="D1" bayName="Q01" cNodeName="CN2"/>
			</TransformerWinding>
		</PowerTransformer>
		<VoltageLevel name="E1" desc="110 kV">
			<Voltage unit="V" multiplier="k">110</Voltage>
			<Bay name="Q01" desc="Transformer T1 HV">
				<LNode iedName="AA1E1Q01A1" ldInst="LD0" lnClass="LLN0" lnInst=""/>
				<ConductingEquipment name="QA1" type="CBR" desc="HV circuit breaker">
					<LNode iedName="AA1E1Q01A1" ldInst="CTRL" lnClass="XCBR" lnInst="1"/>
					<Terminal name="T1" connectivityNode="AA1/E1/Q01/CN1" substationName="AA1" voltageLevelName="E1" bayName="Q01" cNodeName="CN1"/>
					<Terminal name="T2" connectivityNode="AA1/E1/Q01/CN2" substationName="AA1" voltageLevelName="E1" bayName="Q01" cNodeName="CN2"/>
				</ConductingEquipment>
				<ConnectivityNode name="CN1" pathName="AA1/E1/Q01/CN1"/>
				<ConnectivityNode name="CN2" pathName="AA1/E1/Q01/CN2"/>
			</Bay>
		</VoltageLevel>
		<VoltageLevel name="D1" desc="20 kV">
			<Voltage unit="V" multiplier="k">20</Voltage>
			<Bay name="W1" desc="Busbar 1">
				<LNode iedName="AA1D1W1A1" ldInst="PROT" lnClass="PDIF" lnInst="1"/>
				<ConnectivityNode name="CN1" pathName="AA1/D1/W1/CN1"/>
			</Bay>
			<Bay name="Q01" desc="Transformer T1 LV incomer">
				<ConductingEquipment name="QA1" type="CBR" desc="LV incomer circuit breaker">
					<LNode iedName="AA1E1Q01A1" ldInst="CTRL" lnClass="XCBR" lnInst="2"/>
					<Terminal name="T1" connectivityNode="AA1/D1/Q01/CN2" substationName="AA1" voltageLevelName="D1" bayName="Q01" cNodeName="CN2"/>
					<Terminal name="T2" connectivityNode="AA1/D1/W1/CN1" substationName="AA1" voltageLevelName="D1" bayName="W1" cNodeName="CN1"/>
				</ConductingEquipment>
				<ConnectivityNode name="CN2" pathName="AA1/D1/Q01/CN2"/>
			</Bay>
			<Bay name="Q02" desc="Feeder 1 Riverside">
				<LNode iedName="AA1D1Q02A1" ldInst="PROT" lnClass="PTOC" lnInst="1"/>
				<LNode iedName="AA1D1Q02A1" ldInst="PROT" lnClass="PTOC" lnInst="2"/>
				<LNode iedName="AA1D1Q02A1" ldInst="PROT" lnClass="PTRC" lnInst="1"/>
				<LNode iedName="AA1D1Q02A1" ldInst="MEAS" lnClass="MMXU" lnInst="1"/>
				<LNode iedName="AA1D1W1A1" ldInst="PROT" lnClass="RBRF" lnInst="1"/>
				<ConductingEquipment name="QB1" type="DIS" desc="Busbar disconnector">
					<LNode iedName="AA1D1Q02A1" ldInst="CTRL" lnClass="CSWI" lnInst="2"/>
					<LNode iedName="AA1D1Q02A1" ldInst="CTRL" lnClass="XSWI" lnInst="1"/>
					<LNode iedName="AA1D1Q02A1" ldInst="CTRL" lnClass="CILO" lnInst="2"/>
					<Terminal name="T1" connectivityNode="AA1/D1/W1/CN1" substationName="AA1" voltageLevelName="D1" bayName="W1" cNodeName="CN1"/>
					<Terminal name="T2" connectivityNode="AA1/D1/Q02/CN1" substationName="AA1" voltageLevelName="D1" bayName="Q02" cNodeName="CN1"/>
				</ConductingEquipment>
				<ConductingEquipment name="QA1" type="CBR" desc="Feeder circuit breaker">
					<LNode iedName="AA1D1Q02A1" ldInst="CTRL" lnClass="CSWI" lnInst="1"/>
					<LNode iedName="AA1D1Q02A1" ldInst="CTRL" lnClass="XCBR" lnInst="1"/>
					<LNode iedName="AA1D1Q02A1" ldInst="CTRL" lnClass="CILO" lnInst="1"/>
					<Terminal name="T1" connectivityNode="AA1/D1/Q02/CN1" substationName="AA1" voltageLevelName="D1" bayName="Q02" cNodeName="CN1"/>
					<Terminal name="T2" connectivityNode="AA1/D1/Q02/CN2" substationName="AA1" voltageLevelName="D1" bayName="Q02" cNodeName="CN2"/>
				</ConductingEquipment>
				<ConductingEquipment name="BI1" type="CTR" desc="Current transformer 600/1 A">
					<LNode iedName="AA1D1Q02MU1" ldInst="MU01" prefix="I" lnClass="TCTR" lnInst="1"/>
					<LNode iedName="AA1D1Q02MU1" ldInst="MU01" prefix="I" lnClass="TCTR" lnInst="2"/>
					<LNode iedName="AA1D1Q02MU1" ldInst="MU01" prefix="I" lnClass="TCTR" lnInst="3"/>
					<LNode iedName="AA1D1Q02MU1" ldInst="MU01" prefix="I" lnClass="TCTR" lnInst="4"/>
					<Terminal name="T1" connectivityNode="AA1/D1/Q02/CN2" substationName="AA1" voltageLevelName="D1" bayName="Q02" cNodeName="CN2"/>
				</ConductingEquipment>
				<ConductingEquipment name="BU1" type="VTR" desc="Voltage transformer 20/0.1 kV">
					<LNode iedName="AA1D1Q02MU1" ldInst="MU01" prefix="U" lnClass="TVTR" lnInst="1"/>
					<LNode iedName="AA1D1Q02MU1" ldInst="MU01" prefix="U" lnClass="TVTR" lnInst="2"/>
					<LNode iedName="AA1D1Q02MU1" ldInst="MU01" prefix="U" lnClass="TVTR" lnInst="3"/>
					<LNode iedName="AA1D1Q02MU1" ldInst="MU01" prefix="U" lnClass="TVTR" lnInst="4"/>
					<Terminal name="T1" connectivityNode="AA1/D1/Q02/CN2" substationName="AA1" voltageLevelName="D1" bayName="Q02" cNodeName="CN2"/>
				</ConductingEquipment>
				<ConnectivityNode name="CN1" pathName="AA1/D1/Q02/CN1"/>
				<ConnectivityNode name="CN2" pathName="AA1/D1/Q02/CN2"/>
			</Bay>
			<Bay name="Q03" desc="Feeder 2 Hillcrest">
				<LNode iedName="AA1D1Q03A1" ldInst="PROT" lnClass="PTOC" lnInst="1"/>
				<LNode iedName="AA1D1Q03A1" ldInst="PROT" lnClass="PTOC" lnInst="2"/>
				<LNode iedName="AA1D1Q03A1" ldInst="PROT" lnClass="PTRC" lnInst="1"/>
				<LNode iedName="AA1D1Q03A1" ldInst="MEAS" lnClass="MMXU" lnInst="1"/>
				<LNode iedName="AA1D1W1A1" ldInst="PROT" lnClass="RBRF" lnInst="2"/>
				<ConductingEquipment name="QB1" type="DIS" desc="Busbar disconnector">
					<LNode iedName="AA1D1Q03A1" ldInst="CTRL" lnClass="CSWI" lnInst="2"/>
					<LNode iedName="AA1D1Q03A1" ldInst="CTRL" lnClass="XSWI" lnInst="1"/>
					<LNode iedName="AA1D1Q03A1" ldInst="CTRL" lnClass="CILO" lnInst="2"/>
					<Terminal name="T1" connectivityNode="AA1/D1/W1/CN1" substationName="AA1" voltageLevelName="D1" bayName="W1" cNodeName="CN1"/>
					<Terminal name="T2" connectivityNode="AA1/D1/Q03/CN1" substationName="AA1" voltageLevelName="D1" bayName="Q03" cNodeName="CN1"/>
				</ConductingEquipment>
				<ConductingEquipment name="QA1" type="CBR" desc="Feeder circuit breaker">
					<LNode iedName="AA1D1Q03A1" ldInst="CTRL" lnClass="CSWI" lnInst="1"/>
					<LNode iedName="AA1D1Q03A1" ldInst="CTRL" lnClass="XCBR" lnInst="1"/>
					<LNode iedName="AA1D1Q03A1" ldInst="CTRL" lnClass="CILO" lnInst="1"/>
					<Terminal name="T1" connectivityNode="AA1/D1/Q03/CN1" substationName="AA1" voltageLevelName="D1" bayName="Q03" cNodeName="CN1"/>
					<Terminal name="T2" connectivityNode="AA1/D1/Q03/CN2" substationName="AA1" voltageLevelName="D1" bayName="Q03" cNodeName="CN2"/>
				</ConductingEquipment>
				<ConnectivityNode name="CN1" pathName="AA1/D1/Q03/CN1"/>
				<ConnectivityNode name="CN2" pathName="AA1/D1/Q03/CN2"/>
			</Bay>
		</VoltageLevel>
	</Substation>
	<Communication>
		<SubNetwork name="AA1WA1" type="8-MMS" desc="Station bus (MMS, GOOSE and Sampled Values)">
			<BitRate unit="b/s" multiplier="M">100</BitRate>
			<ConnectedAP iedName="AA1KA1" apName="S1">
				<Address>
					<P type="IP" xsi:type="tP_IP">10.20.1.10</P>
					<P type="IP-SUBNET" xsi:type="tP_IP-SUBNET">255.255.255.0</P>
					<P type="IP-GATEWAY" xsi:type="tP_IP-GATEWAY">10.20.1.1</P>
					<P type="OSI-AP-Title" xsi:type="tP_OSI-AP-Title">1,3,9999,33</P>
					<P type="OSI-AE-Qualifier" xsi:type="tP_OSI-AE-Qualifier">33</P>
					<P type="OSI-PSEL" xsi:type="tP_OSI-PSEL">00000001</P>
					<P type="OSI-SSEL" xsi:type="tP_OSI-SSEL">0001</P>
					<P type="OSI-TSEL" xsi:type="tP_OSI-TSEL">0001</P>
				</Address>
				<PhysConn type="Connection">
					<P type="Type" xsi:type="tP_Type">FOC</P>
					<P type="Plug" xsi:type="tP_Plug">LC</P>
				</PhysConn>
			</ConnectedAP>
			<ConnectedAP iedName="AA1E1Q01A1" apName="S1">
				<Address>
					<P type="IP" xsi:type="tP_IP">10.20.1.21</P>
					<P type="IP-SUBNET" xsi:type="tP_IP-SUBNET">255.255.255.0</P>
					<P type="IP-GATEWAY" xsi:type="tP_IP-GATEWAY">10.20.1.1</P>
					<P type="OSI-AP-Title" xsi:type="tP_OSI-AP-Title">1,3,9999,33</P>
					<P type="OSI-AE-Qualifier" xsi:type="tP_OSI-AE-Qualifier">33</P>
					<P type="OSI-PSEL" xsi:type="tP_OSI-PSEL">00000001</P>
					<P type="OSI-SSEL" xsi:type="tP_OSI-SSEL">0001</P>
					<P type="OSI-TSEL" xsi:type="tP_OSI-TSEL">0001</P>
				</Address>
				<PhysConn type="Connection">
					<P type="Type" xsi:type="tP_Type">FOC</P>
					<P type="Plug" xsi:type="tP_Plug">LC</P>
				</PhysConn>
			</ConnectedAP>
			<ConnectedAP iedName="AA1D1Q02A1" apName="S1">
				<Address>
					<P type="IP" xsi:type="tP_IP">10.20.1.31</P>
					<P type="IP-SUBNET" xsi:type="tP_IP-SUBNET">255.255.255.0</P>
					<P type="IP-GATEWAY" xsi:type="tP_IP-GATEWAY">10.20.1.1</P>
					<P type="OSI-AP-Title" xsi:type="tP_OSI-AP-Title">1,3,9999,33</P>
					<P type="OSI-AE-Qualifier" xsi:type="tP_OSI-AE-Qualifier">33</P>
					<P type="OSI-PSEL" xsi:type="tP_OSI-PSEL">00000001</P>
					<P type="OSI-SSEL" xsi:type="tP_OSI-SSEL">0001</P>
					<P type="OSI-TSEL" xsi:type="tP_OSI-TSEL">0001</P>
				</Address>
				<GSE ldInst="PROT" cbName="gcbTrip">
					<Address>
						<P type="MAC-Address" xsi:type="tP_MAC-Address">01-0C-CD-01-00-11</P>
						<P type="APPID" xsi:type="tP_APPID">0101</P>
						<P type="VLAN-ID" xsi:type="tP_VLAN-ID">00A</P>
						<P type="VLAN-PRIORITY" xsi:type="tP_VLAN-PRIORITY">4</P>
					</Address>
					<MinTime unit="s" multiplier="m">4</MinTime>
					<MaxTime unit="s" multiplier="m">1000</MaxTime>
				</GSE>
				<GSE ldInst="CTRL" cbName="gcbPos">
					<Address>
						<P type="MAC-Address" xsi:type="tP_MAC-Address">01-0C-CD-01-00-12</P>
						<P type="APPID" xsi:type="tP_APPID">0102</P>
						<P type="VLAN-ID" xsi:type="tP_VLAN-ID">00A</P>
						<P type="VLAN-PRIORITY" xsi:type="tP_VLAN-PRIORITY">4</P>
					</Address>
					<MinTime unit="s" multiplier="m">4</MinTime>
					<MaxTime unit="s" multiplier="m">1000</MaxTime>
				</GSE>
				<PhysConn type="Connection">
					<P type="Type" xsi:type="tP_Type">FOC</P>
					<P type="Plug" xsi:type="tP_Plug">LC</P>
				</PhysConn>
			</ConnectedAP>
			<ConnectedAP iedName="AA1D1Q03A1" apName="S1">
				<Address>
					<P type="IP" xsi:type="tP_IP">10.20.1.32</P>
					<P type="IP-SUBNET" xsi:type="tP_IP-SUBNET">255.255.255.0</P>
					<P type="IP-GATEWAY" xsi:type="tP_IP-GATEWAY">10.20.1.1</P>
					<P type="OSI-AP-Title" xsi:type="tP_OSI-AP-Title">1,3,9999,33</P>
					<P type="OSI-AE-Qualifier" xsi:type="tP_OSI-AE-Qualifier">33</P>
					<P type="OSI-PSEL" xsi:type="tP_OSI-PSEL">00000001</P>
					<P type="OSI-SSEL" xsi:type="tP_OSI-SSEL">0001</P>
					<P type="OSI-TSEL" xsi:type="tP_OSI-TSEL">0001</P>
				</Address>
				<GSE ldInst="PROT" cbName="gcbTrip">
					<Address>
						<P type="MAC-Address" xsi:type="tP_MAC-Address">01-0C-CD-01-00-21</P>
						<P type="APPID" xsi:type="tP_APPID">0201</P>
						<P type="VLAN-ID" xsi:type="tP_VLAN-ID">00A</P>
						<P type="VLAN-PRIORITY" xsi:type="tP_VLAN-PRIORITY">4</P>
					</Address>
					<MinTime unit="s" multiplier="m">4</MinTime>
					<MaxTime unit="s" multiplier="m">1000</MaxTime>
				</GSE>
				<GSE ldInst="CTRL" cbName="gcbPos">
					<Address>
						<P type="MAC-Address" xsi:type="tP_MAC-Address">01-0C-CD-01-00-22</P>
						<P type="APPID" xsi:type="tP_APPID">0202</P>
						<P type="VLAN-ID" xsi:type="tP_VLAN-ID">00A</P>
						<P type="VLAN-PRIORITY" xsi:type="tP_VLAN-PRIORITY">4</P>
					</Address>
					<MinTime unit="s" multiplier="m">4</MinTime>
					<MaxTime unit="s" multiplier="m">1000</MaxTime>
				</GSE>
				<PhysConn type="Connection">
					<P type="Type" xsi:type="tP_Type">FOC</P>
					<P type="Plug" xsi:type="tP_Plug">LC</P>
				</PhysConn>
			</ConnectedAP>
			<ConnectedAP iedName="AA1D1W1A1" apName="S1">
				<Address>
					<P type="IP" xsi:type="tP_IP">10.20.1.41</P>
					<P type="IP-SUBNET" xsi:type="tP_IP-SUBNET">255.255.255.0</P>
					<P type="IP-GATEWAY" xsi:type="tP_IP-GATEWAY">10.20.1.1</P>
					<P type="OSI-AP-Title" xsi:type="tP_OSI-AP-Title">1,3,9999,33</P>
					<P type="OSI-AE-Qualifier" xsi:type="tP_OSI-AE-Qualifier">33</P>
					<P type="OSI-PSEL" xsi:type="tP_OSI-PSEL">00000001</P>
					<P type="OSI-SSEL" xsi:type="tP_OSI-SSEL">0001</P>
					<P type="OSI-TSEL" xsi:type="tP_OSI-TSEL">0001</P>
				</Address>
				<GSE ldInst="PROT" cbName="gcbBF">
					<Address>
						<P type="MAC-Address" xsi:type="tP_MAC-Address">01-0C-CD-01-00-41</P>
						<P type="APPID" xsi:type="tP_APPID">0401</P>
						<P type="VLAN-ID" xsi:type="tP_VLAN-ID">00A</P>
						<P type="VLAN-PRIORITY" xsi:type="tP_VLAN-PRIORITY">6</P>
					</Address>
					<MinTime unit="s" multiplier="m">2</MinTime>
					<MaxTime unit="s" multiplier="m">1000</MaxTime>
				</GSE>
				<PhysConn type="Connection">
					<P type="Type" xsi:type="tP_Type">FOC</P>
					<P type="Plug" xsi:type="tP_Plug">LC</P>
				</PhysConn>
			</ConnectedAP>
			<ConnectedAP iedName="AA1D1Q02MU1" apName="S1">
				<Address>
					<P type="IP" xsi:type="tP_IP">10.20.1.51</P>
					<P type="IP-SUBNET" xsi:type="tP_IP-SUBNET">255.255.255.0</P>
					<P type="IP-GATEWAY" xsi:type="tP_IP-GATEWAY">10.20.1.1</P>
					<P type="OSI-AP-Title" xsi:type="tP_OSI-AP-Title">1,3,9999,33</P>
					<P type="OSI-AE-Qualifier" xsi:type="tP_OSI-AE-Qualifier">33</P>
					<P type="OSI-PSEL" xsi:type="tP_OSI-PSEL">00000001</P>
					<P type="OSI-SSEL" xsi:type="tP_OSI-SSEL">0001</P>
					<P type="OSI-TSEL" xsi:type="tP_OSI-TSEL">0001</P>
				</Address>
				<GSE ldInst="MU01" cbName="gcbStatus">
					<Address>
						<P type="MAC-Address" xsi:type="tP_MAC-Address">01-0C-CD-01-00-51</P>
						<P type="APPID" xsi:type="tP_APPID">0501</P>
						<P type="VLAN-ID" xsi:type="tP_VLAN-ID">00A</P>
						<P type="VLAN-PRIORITY" xsi:type="tP_VLAN-PRIORITY">4</P>
					</Address>
					<MinTime unit="s" multiplier="m">4</MinTime>
					<MaxTime unit="s" multiplier="m">1000</MaxTime>
				</GSE>
				<SMV ldInst="MU01" cbName="MSVCB01">
					<Address>
						<P type="MAC-Address" xsi:type="tP_MAC-Address">01-0C-CD-04-00-01</P>
						<P type="APPID" xsi:type="tP_APPID">4001</P>
						<P type="VLAN-ID" xsi:type="tP_VLAN-ID">014</P>
						<P type="VLAN-PRIORITY" xsi:type="tP_VLAN-PRIORITY">4</P>
					</Address>
				</SMV>
				<PhysConn type="Connection">
					<P type="Type" xsi:type="tP_Type">FOC</P>
					<P type="Plug" xsi:type="tP_Plug">LC</P>
				</PhysConn>
			</ConnectedAP>
		</SubNetwork>
	</Communication>
	<IED name="AA1KA1" desc="Station HMI and SCADA gateway" type="Station computer" manufacturer="OT Fabric Demo" configVersion="1.2" originalSclVersion="2007" originalSclRevision="B" originalSclRelease="4">
		<Services>
			<GetDirectory/>
			<GetDataObjectDefinition/>
			<GetDataSetValue/>
			<DataSetDirectory/>
			<ReadWrite/>
			<GetCBValues/>
			<FileHandling/>
			<ClientServices goose="false" gsse="false" bufReport="true" unbufReport="true" readLog="true" sv="false"/>
		</Services>
		<AccessPoint name="S1" desc="Station bus">
			<Server>
				<Authentication none="true"/>
				<LDevice inst="LD0" desc="Station HMI">
					<LN0 lnClass="LLN0" inst="" lnType="LLN0_A">
						<DOI name="NamPlt">
							<DAI name="vendor">
								<Val>OT Fabric Demo</Val>
							</DAI>
							<DAI name="swRev">
								<Val>2.4.0</Val>
							</DAI>
							<DAI name="d">
								<Val>Station HMI</Val>
							</DAI>
							<DAI name="configRev">
								<Val>AA1-2026-11-03</Val>
							</DAI>
						</DOI>
					</LN0>
					<LN lnClass="LPHD" inst="1" lnType="LPHD_A">
						<DOI name="PhyNam">
							<DAI name="vendor">
								<Val>OT Fabric Demo</Val>
							</DAI>
							<DAI name="hwRev">
								<Val>C</Val>
							</DAI>
							<DAI name="swRev">
								<Val>2.4.0</Val>
							</DAI>
							<DAI name="serNum">
								<Val>KA200-0019</Val>
							</DAI>
							<DAI name="model">
								<Val>KA-200 station computer</Val>
							</DAI>
							<DAI name="location">
								<Val>AA1 Northfield, relay room</Val>
							</DAI>
						</DOI>
					</LN>
					<LN lnClass="IHMI" inst="1" lnType="IHMI_A" desc="Operator workplace"/>
				</LDevice>
			</Server>
		</AccessPoint>
	</IED>
	<IED name="AA1E1Q01A1" desc="Transformer T1 protection and control" type="TPC-640" manufacturer="OT Fabric Demo" configVersion="2.0" originalSclVersion="2007" originalSclRevision="B" originalSclRelease="4">
		<Services>
			<DynAssociation max="4"/>
			<SettingGroups><SGEdit/><ConfSG/></SettingGroups>
			<GetDirectory/>
			<GetDataObjectDefinition/>
			<DataObjectDirectory/>
			<GetDataSetValue/>
			<DataSetDirectory/>
			<ConfDataSet max="12" maxAttributes="64"/>
			<ReadWrite/>
			<ConfReportControl max="12"/>
			<GetCBValues/>
			<ReportSettings rptID="Dyn" optFields="Dyn" bufTime="Dyn" trgOps="Dyn" intgPd="Dyn"/>
			<GSESettings appID="Conf" dataLabel="Fix"/>
			<GOOSE max="4"/>
			<ConfLogControl max="2"/>
			<FileHandling/>
			<ConfLNs fixPrefix="true" fixLnInst="true"/>
		</Services>
		<AccessPoint name="S1" desc="Station bus">
			<Server>
				<Authentication none="true"/>
				<LDevice inst="LD0" desc="Device">
					<LN0 lnClass="LLN0" inst="" lnType="LLN0_A">
						<DOI name="NamPlt">
							<DAI name="vendor">
								<Val>OT Fabric Demo</Val>
							</DAI>
							<DAI name="swRev">
								<Val>2.4.0</Val>
							</DAI>
							<DAI name="d">
								<Val>Transformer T1</Val>
							</DAI>
							<DAI name="configRev">
								<Val>AA1-2026-11-03</Val>
							</DAI>
						</DOI>
					</LN0>
					<LN lnClass="LPHD" inst="1" lnType="LPHD_A">
						<DOI name="PhyNam">
							<DAI name="vendor">
								<Val>OT Fabric Demo</Val>
							</DAI>
							<DAI name="hwRev">
								<Val>C</Val>
							</DAI>
							<DAI name="swRev">
								<Val>2.4.0</Val>
							</DAI>
							<DAI name="serNum">
								<Val>TPC640-260311</Val>
							</DAI>
							<DAI name="model">
								<Val>TPC-640</Val>
							</DAI>
							<DAI name="location">
								<Val>AA1 Northfield, relay room</Val>
							</DAI>
						</DOI>
					</LN>
				</LDevice>
				<LDevice inst="PROT" desc="Protection">
					<LN0 lnClass="LLN0" inst="" lnType="LLN0_A">
						<DataSet name="dsTrip" desc="Protection starts and trips">
							<FCDA ldInst="PROT" lnClass="PDIF" lnInst="1" doName="Str" fc="ST"/>
							<FCDA ldInst="PROT" lnClass="PDIF" lnInst="1" doName="Op" fc="ST"/>
							<FCDA ldInst="PROT" lnClass="PTOC" lnInst="1" doName="Str" fc="ST"/>
							<FCDA ldInst="PROT" lnClass="PTOC" lnInst="1" doName="Op" fc="ST"/>
							<FCDA ldInst="PROT" lnClass="PTRC" lnInst="1" doName="Tr" fc="ST"/>
						</DataSet>
						<ReportControl name="brcbTrip" desc="Protection events to the station HMI" datSet="dsTrip" rptID="AA1E1Q01A1PROT/LLN0.brcbTrip" confRev="1" buffered="true" bufTime="50" indexed="false">
							<TrgOps dchg="true" qchg="true" gi="true"/>
							<OptFields seqNum="true" dataSet="true" reasonCode="true" dataRef="true" entryID="true" configRef="true" bufOvfl="false"/>
							<RptEnabled max="2">
								<ClientLN iedName="AA1KA1" ldInst="LD0" lnClass="IHMI" lnInst="1" apRef="S1"/>
							</RptEnabled>
						</ReportControl>
						<DOI name="NamPlt">
							<DAI name="vendor">
								<Val>OT Fabric Demo</Val>
							</DAI>
							<DAI name="swRev">
								<Val>2.4.0</Val>
							</DAI>
							<DAI name="d">
								<Val>Protection</Val>
							</DAI>
							<DAI name="configRev">
								<Val>AA1-2026-11-03</Val>
							</DAI>
						</DOI>
					</LN0>
					<LN lnClass="PDIF" inst="1" lnType="PDIF_A" desc="Transformer differential (87T)">
						<DOI name="LoSet">
							<SDI name="setMag">
								<DAI name="f">
									<Val sGroup="1">0.3</Val>
								</DAI>
							</SDI>
						</DOI>
					</LN>
					<LN lnClass="PTOC" inst="1" lnType="PTOC_A" desc="LV overcurrent (51), blocked by feeder starts">
						<DOI name="StrVal">
							<SDI name="setMag">
								<DAI name="f">
									<Val sGroup="1">1400</Val>
								</DAI>
							</SDI>
						</DOI>
						<DOI name="OpDlTmms">
							<DAI name="setVal">
								<Val sGroup="1">700</Val>
							</DAI>
						</DOI>
						<Inputs>
							<ExtRef desc="Feeder 1 trip blocks LV overcurrent" iedName="AA1D1Q02A1" ldInst="PROT" lnClass="PTRC" lnInst="1" doName="Tr" daName="general" intAddr="BlkF1" serviceType="GOOSE" srcLDInst="PROT" srcLNClass="LLN0" srcCBName="gcbTrip"/>
							<ExtRef desc="Feeder 2 trip blocks LV overcurrent" iedName="AA1D1Q03A1" ldInst="PROT" lnClass="PTRC" lnInst="1" doName="Tr" daName="general" intAddr="BlkF2" serviceType="GOOSE" srcLDInst="PROT" srcLNClass="LLN0" srcCBName="gcbTrip"/>
						</Inputs>
					</LN>
					<LN lnClass="PTRC" inst="1" lnType="PTRC_A" desc="Trip conditioning">
						<Inputs>
							<ExtRef desc="Breaker failure feeder 1: trip LV breaker" iedName="AA1D1W1A1" ldInst="PROT" lnClass="RBRF" lnInst="1" doName="OpEx" daName="general" intAddr="BFTrip1" serviceType="GOOSE" srcLDInst="PROT" srcLNClass="LLN0" srcCBName="gcbBF"/>
							<ExtRef desc="Breaker failure feeder 2: trip LV breaker" iedName="AA1D1W1A1" ldInst="PROT" lnClass="RBRF" lnInst="2" doName="OpEx" daName="general" intAddr="BFTrip2" serviceType="GOOSE" srcLDInst="PROT" srcLNClass="LLN0" srcCBName="gcbBF"/>
						</Inputs>
					</LN>
				</LDevice>
				<LDevice inst="CTRL" desc="Control">
					<LN0 lnClass="LLN0" inst="" lnType="LLN0_A">
						<DataSet name="dsTap" desc="Tap changer and breakers">
							<FCDA ldInst="CTRL" lnClass="YLTC" lnInst="1" doName="TapPos" fc="ST"/>
							<FCDA ldInst="CTRL" lnClass="ATCC" lnInst="1" doName="Auto" fc="ST"/>
							<FCDA ldInst="CTRL" lnClass="XCBR" lnInst="1" doName="Pos" fc="ST"/>
							<FCDA ldInst="CTRL" lnClass="XCBR" lnInst="2" doName="Pos" fc="ST"/>
							<FCDA ldInst="CTRL" lnClass="YLTC" lnInst="1" doName="MotDrvTmp" fc="MX"/>
						</DataSet>
						<ReportControl name="urcbTap" desc="Tap changer to the station HMI" datSet="dsTap" rptID="AA1E1Q01A1CTRL/LLN0.urcbTap" confRev="1" buffered="false" bufTime="100" indexed="false">
							<TrgOps dchg="true" qchg="true" gi="true"/>
							<OptFields seqNum="true" dataSet="true" reasonCode="true" configRef="true" bufOvfl="false"/>
							<RptEnabled max="2">
								<ClientLN iedName="AA1KA1" ldInst="LD0" lnClass="IHMI" lnInst="1" apRef="S1"/>
							</RptEnabled>
						</ReportControl>
						<DOI name="NamPlt">
							<DAI name="vendor">
								<Val>OT Fabric Demo</Val>
							</DAI>
							<DAI name="swRev">
								<Val>2.4.0</Val>
							</DAI>
							<DAI name="d">
								<Val>Control</Val>
							</DAI>
							<DAI name="configRev">
								<Val>AA1-2026-11-03</Val>
							</DAI>
						</DOI>
					</LN0>
					<LN lnClass="YPTR" inst="1" lnType="YPTR_A" desc="Power transformer T1 110/20 kV 40 MVA"/>
					<LN lnClass="YLTC" inst="1" lnType="YLTC_CV" desc="On-load tap changer">
						<DOI name="NamPlt">
							<DAI name="vendor">
								<Val>Corvane Energy (fictitious)</Val>
							</DAI>
							<DAI name="swRev">
								<Val>5.2</Val>
							</DAI>
							<DAI name="d">
								<Val>Tap changer with motor drive supervision</Val>
							</DAI>
							<DAI name="lnNs">
								<Val>CORVANE 7-4X:2026A</Val>
							</DAI>
						</DOI>
					</LN>
					<LN lnClass="ATCC" inst="1" lnType="ATCC_A" desc="Automatic voltage control">
						<DOI name="BndCtr">
							<SDI name="setMag">
								<DAI name="f">
									<Val sGroup="1">20.5</Val>
								</DAI>
							</SDI>
						</DOI>
						<DOI name="BndWid">
							<SDI name="setMag">
								<DAI name="f">
									<Val sGroup="1">1.5</Val>
								</DAI>
							</SDI>
						</DOI>
					</LN>
					<LN lnClass="XCBR" inst="1" lnType="XCBR_A" desc="HV circuit breaker E1Q01 QA1"/>
					<LN lnClass="XCBR" inst="2" lnType="XCBR_A" desc="LV incomer circuit breaker D1Q01 QA1"/>
				</LDevice>
				<LDevice inst="MEAS" desc="Measurements">
					<LN0 lnClass="LLN0" inst="" lnType="LLN0_A">
						<DataSet name="dsMeas" desc="Transformer measurements">
							<FCDA ldInst="MEAS" lnClass="MMXU" lnInst="1" doName="TotW" fc="MX"/>
							<FCDA ldInst="MEAS" lnClass="MMXU" lnInst="1" doName="A" fc="MX"/>
							<FCDA ldInst="MEAS" lnClass="MMXU" lnInst="2" doName="TotW" fc="MX"/>
							<FCDA ldInst="MEAS" lnClass="MMXU" lnInst="2" doName="Hz" fc="MX"/>
							<FCDA ldInst="MEAS" lnClass="MMXU" lnInst="2" doName="A" fc="MX"/>
						</DataSet>
						<ReportControl name="urcbMeas" desc="Measurements to the station HMI" datSet="dsMeas" intgPd="10000" rptID="AA1E1Q01A1MEAS/LLN0.urcbMeas" confRev="1" buffered="false" bufTime="100" indexed="false">
							<TrgOps dchg="true" period="true" gi="true"/>
							<OptFields seqNum="true" dataSet="true" reasonCode="true" configRef="true" bufOvfl="false"/>
							<RptEnabled max="2">
								<ClientLN iedName="AA1KA1" ldInst="LD0" lnClass="IHMI" lnInst="1" apRef="S1"/>
							</RptEnabled>
						</ReportControl>
						<DOI name="NamPlt">
							<DAI name="vendor">
								<Val>OT Fabric Demo</Val>
							</DAI>
							<DAI name="swRev">
								<Val>2.4.0</Val>
							</DAI>
							<DAI name="d">
								<Val>Measurements</Val>
							</DAI>
							<DAI name="configRev">
								<Val>AA1-2026-11-03</Val>
							</DAI>
						</DOI>
					</LN0>
					<LN lnClass="MMXU" inst="1" lnType="MMXU_A" desc="HV side"/>
					<LN lnClass="MMXU" inst="2" lnType="MMXU_A" desc="LV side"/>
					<LN lnClass="MMXN" inst="1" lnType="MMXN_A" desc="Tap changer motor supply, 230 V"/>
					<LN lnClass="MHAI" inst="1" lnType="MHAI_A" desc="Harmonics, LV side"/>
					<LN lnClass="MMTR" inst="1" lnType="MMTR_A" desc="Transformer energy meter, LV side"/>
				</LDevice>
			</Server>
		</AccessPoint>
	</IED>
	<IED name="AA1D1Q02A1" desc="Feeder 1 protection and control (Riverside)" type="FPC-615" manufacturer="OT Fabric Demo" configVersion="3.1" originalSclVersion="2007" originalSclRevision="B" originalSclRelease="4">
		<Services>
			<DynAssociation max="4"/>
			<SettingGroups><SGEdit/><ConfSG/></SettingGroups>
			<GetDirectory/>
			<GetDataObjectDefinition/>
			<DataObjectDirectory/>
			<GetDataSetValue/>
			<DataSetDirectory/>
			<ConfDataSet max="12" maxAttributes="64"/>
			<ReadWrite/>
			<ConfReportControl max="12"/>
			<GetCBValues/>
			<ReportSettings rptID="Dyn" optFields="Dyn" bufTime="Dyn" trgOps="Dyn" intgPd="Dyn"/>
			<GSESettings appID="Conf" dataLabel="Fix"/>
			<GOOSE max="4"/>
			<ConfLogControl max="2"/>
			<FileHandling/>
			<ConfLNs fixPrefix="true" fixLnInst="true"/>
		</Services>
		<AccessPoint name="S1" desc="Station bus">
			<Server>
				<Authentication none="true"/>
				<LDevice inst="LD0" desc="Device">
					<LN0 lnClass="LLN0" inst="" lnType="LLN0_A">
						<DOI name="NamPlt">
							<DAI name="vendor">
								<Val>OT Fabric Demo</Val>
							</DAI>
							<DAI name="swRev">
								<Val>2.4.0</Val>
							</DAI>
							<DAI name="d">
								<Val>Feeder 1 protection and control</Val>
							</DAI>
							<DAI name="configRev">
								<Val>AA1-2026-11-03</Val>
							</DAI>
						</DOI>
					</LN0>
					<LN lnClass="LPHD" inst="1" lnType="LPHD_A">
						<DOI name="PhyNam">
							<DAI name="vendor">
								<Val>OT Fabric Demo</Val>
							</DAI>
							<DAI name="hwRev">
								<Val>C</Val>
							</DAI>
							<DAI name="swRev">
								<Val>2.4.0</Val>
							</DAI>
							<DAI name="serNum">
								<Val>FPC-615-264411</Val>
							</DAI>
							<DAI name="model">
								<Val>FPC-615</Val>
							</DAI>
							<DAI name="location">
								<Val>AA1 Northfield, relay room</Val>
							</DAI>
						</DOI>
					</LN>
					<LN lnClass="GGIO" inst="1" lnType="GGIO_IND3" desc="Breaker supervision inputs"/>
				</LDevice>
				<LDevice inst="PROT" desc="Protection">
					<LN0 lnClass="LLN0" inst="" lnType="LLN0_A">
						<DataSet name="dsTrip" desc="Protection starts and trips">
							<FCDA ldInst="PROT" lnClass="PTOC" lnInst="1" doName="Str" fc="ST"/>
							<FCDA ldInst="PROT" lnClass="PTOC" lnInst="1" doName="Op" fc="ST"/>
							<FCDA ldInst="PROT" lnClass="PTOC" lnInst="2" doName="Str" fc="ST"/>
							<FCDA ldInst="PROT" lnClass="PTOC" lnInst="2" doName="Op" fc="ST"/>
							<FCDA ldInst="PROT" lnClass="PTRC" lnInst="1" doName="Tr" fc="ST"/>
						</DataSet>
						<DataSet name="dsGoTrip" desc="Trip for GOOSE">
							<FCDA ldInst="PROT" lnClass="PTRC" lnInst="1" doName="Tr" daName="general" fc="ST"/>
							<FCDA ldInst="PROT" lnClass="PTRC" lnInst="1" doName="Tr" daName="q" fc="ST"/>
						</DataSet>
						<ReportControl name="brcbTrip" desc="Protection events to the station HMI" datSet="dsTrip" rptID="AA1D1Q02A1PROT/LLN0.brcbTrip" confRev="1" buffered="true" bufTime="50" indexed="false">
							<TrgOps dchg="true" qchg="true" gi="true"/>
							<OptFields seqNum="true" dataSet="true" reasonCode="true" dataRef="true" entryID="true" configRef="true" bufOvfl="false"/>
							<RptEnabled max="2">
								<ClientLN iedName="AA1KA1" ldInst="LD0" lnClass="IHMI" lnInst="1" apRef="S1"/>
							</RptEnabled>
						</ReportControl>
						<DOI name="NamPlt">
							<DAI name="vendor">
								<Val>OT Fabric Demo</Val>
							</DAI>
							<DAI name="swRev">
								<Val>2.4.0</Val>
							</DAI>
							<DAI name="d">
								<Val>Protection</Val>
							</DAI>
							<DAI name="configRev">
								<Val>AA1-2026-11-03</Val>
							</DAI>
						</DOI>
						<GSEControl name="gcbTrip" desc="Trip to busbar and transformer protection" datSet="dsGoTrip" confRev="1" appID="AA1D1Q02A1_Trip" type="GOOSE"/>
						<SettingControl numOfSGs="2" actSG="1"/>
					</LN0>
					<LN lnClass="PTOC" inst="1" lnType="PTOC_A" desc="Overcurrent I&gt; (51)">
						<DOI name="StrVal">
							<SDI name="setMag">
								<DAI name="f">
									<Val sGroup="1">600</Val>
									<Val sGroup="2">800</Val>
								</DAI>
							</SDI>
							<SDI name="minVal">
								<DAI name="f">
									<Val>50</Val>
								</DAI>
							</SDI>
							<SDI name="maxVal">
								<DAI name="f">
									<Val>5000</Val>
								</DAI>
							</SDI>
							<SDI name="stepSize">
								<DAI name="f">
									<Val>10</Val>
								</DAI>
							</SDI>
						</DOI>
						<DOI name="OpDlTmms">
							<DAI name="setVal">
								<Val sGroup="1">300</Val>
								<Val sGroup="2">500</Val>
							</DAI>
							<DAI name="minVal">
								<Val>0</Val>
							</DAI>
							<DAI name="maxVal">
								<Val>60000</Val>
							</DAI>
							<DAI name="stepSize">
								<Val>10</Val>
							</DAI>
						</DOI>
					</LN>
					<LN lnClass="PTOC" inst="2" lnType="PTOC_A" desc="Overcurrent I&gt;&gt; (50)">
						<DOI name="StrVal">
							<SDI name="setMag">
								<DAI name="f">
									<Val sGroup="1">4000</Val>
									<Val sGroup="2">5000</Val>
								</DAI>
							</SDI>
							<SDI name="minVal">
								<DAI name="f">
									<Val>100</Val>
								</DAI>
							</SDI>
							<SDI name="maxVal">
								<DAI name="f">
									<Val>20000</Val>
								</DAI>
							</SDI>
							<SDI name="stepSize">
								<DAI name="f">
									<Val>10</Val>
								</DAI>
							</SDI>
						</DOI>
						<DOI name="OpDlTmms">
							<DAI name="setVal">
								<Val sGroup="1">50</Val>
								<Val sGroup="2">50</Val>
							</DAI>
							<DAI name="minVal">
								<Val>0</Val>
							</DAI>
							<DAI name="maxVal">
								<Val>60000</Val>
							</DAI>
							<DAI name="stepSize">
								<Val>10</Val>
							</DAI>
						</DOI>
					</LN>
					<LN lnClass="PTRC" inst="1" lnType="PTRC_A" desc="Trip conditioning"/>
				</LDevice>
				<LDevice inst="CTRL" desc="Control">
					<LN0 lnClass="LLN0" inst="" lnType="LLN0_A">
						<DataSet name="dsPos" desc="Switch positions and interlocking">
							<FCDA ldInst="CTRL" lnClass="CSWI" lnInst="1" doName="Pos" fc="ST"/>
							<FCDA ldInst="CTRL" lnClass="CSWI" lnInst="2" doName="Pos" fc="ST"/>
							<FCDA ldInst="CTRL" lnClass="XCBR" lnInst="1" doName="Pos" fc="ST"/>
							<FCDA ldInst="CTRL" lnClass="XSWI" lnInst="1" doName="Pos" fc="ST"/>
							<FCDA ldInst="CTRL" lnClass="XCBR" lnInst="1" doName="OpCnt" fc="ST"/>
							<FCDA ldInst="CTRL" lnClass="CILO" lnInst="2" doName="EnaOpn" daName="stVal" fc="ST"/>
							<FCDA ldInst="CTRL" lnClass="CILO" lnInst="2" doName="EnaCls" daName="stVal" fc="ST"/>
						</DataSet>
						<DataSet name="dsGoPos" desc="Breaker position for GOOSE">
							<FCDA ldInst="CTRL" lnClass="XCBR" lnInst="1" doName="Pos" daName="stVal" fc="ST"/>
							<FCDA ldInst="CTRL" lnClass="XCBR" lnInst="1" doName="Pos" daName="q" fc="ST"/>
							<FCDA ldInst="CTRL" lnClass="XCBR" lnInst="1" doName="Pos" daName="t" fc="ST"/>
						</DataSet>
						<ReportControl name="brcbPos" desc="Switch positions to the station HMI" datSet="dsPos" rptID="AA1D1Q02A1CTRL/LLN0.brcbPos" confRev="1" buffered="true" bufTime="50" indexed="false">
							<TrgOps dchg="true" qchg="true" gi="true"/>
							<OptFields seqNum="true" dataSet="true" reasonCode="true" entryID="true" configRef="true" bufOvfl="false"/>
							<RptEnabled max="2">
								<ClientLN iedName="AA1KA1" ldInst="LD0" lnClass="IHMI" lnInst="1" apRef="S1"/>
							</RptEnabled>
						</ReportControl>
						<LogControl name="lcbEvents" desc="Switching events" datSet="dsPos" logName="EventLog" logEna="true" reasonCode="true">
							<TrgOps dchg="true" qchg="true"/>
						</LogControl>
						<DOI name="NamPlt">
							<DAI name="vendor">
								<Val>OT Fabric Demo</Val>
							</DAI>
							<DAI name="swRev">
								<Val>2.4.0</Val>
							</DAI>
							<DAI name="d">
								<Val>Control</Val>
							</DAI>
							<DAI name="configRev">
								<Val>AA1-2026-11-03</Val>
							</DAI>
						</DOI>
						<Log name="EventLog"/>
						<GSEControl name="gcbPos" desc="Breaker position to the busbar protection" datSet="dsGoPos" confRev="1" appID="AA1D1Q02A1_Pos" type="GOOSE"/>
					</LN0>
					<LN lnClass="CSWI" inst="1" lnType="CSWI_SBOE" desc="Circuit breaker QA1 control"/>
					<LN lnClass="CSWI" inst="2" lnType="CSWI_SBOE" desc="Disconnector QB1 control"/>
					<LN lnClass="XCBR" inst="1" lnType="XCBR_A" desc="Circuit breaker QA1"/>
					<LN lnClass="XSWI" inst="1" lnType="XSWI_A" desc="Disconnector QB1">
						<DOI name="SwTyp">
							<DAI name="stVal">
								<Val>Disconnector</Val>
							</DAI>
						</DOI>
					</LN>
					<LN lnClass="CILO" inst="1" lnType="CILO_A" desc="Interlocking QA1"/>
					<LN lnClass="CILO" inst="2" lnType="CILO_A" desc="Interlocking QB1 (open QA1 first)"/>
				</LDevice>
				<LDevice inst="MEAS" desc="Measurements">
					<LN0 lnClass="LLN0" inst="" lnType="LLN0_A">
						<DataSet name="dsMeas" desc="Feeder measurements">
							<FCDA ldInst="MEAS" lnClass="MMXU" lnInst="1" doName="TotW" fc="MX"/>
							<FCDA ldInst="MEAS" lnClass="MMXU" lnInst="1" doName="TotVAr" fc="MX"/>
							<FCDA ldInst="MEAS" lnClass="MMXU" lnInst="1" doName="Hz" fc="MX"/>
							<FCDA ldInst="MEAS" lnClass="MMXU" lnInst="1" doName="A" fc="MX"/>
							<FCDA ldInst="MEAS" lnClass="MMXU" lnInst="1" doName="PhV" fc="MX"/>
							<FCDA ldInst="MEAS" lnClass="MSQI" lnInst="1" doName="SeqA" fc="MX"/>
						</DataSet>
						<ReportControl name="urcbMeas" desc="Measurements to the station HMI" datSet="dsMeas" intgPd="10000" rptID="AA1D1Q02A1MEAS/LLN0.urcbMeas" confRev="1" buffered="false" bufTime="100" indexed="false">
							<TrgOps dchg="true" qchg="true" period="true" gi="true"/>
							<OptFields seqNum="true" dataSet="true" reasonCode="true" configRef="true" bufOvfl="false"/>
							<RptEnabled max="2">
								<ClientLN iedName="AA1KA1" ldInst="LD0" lnClass="IHMI" lnInst="1" apRef="S1"/>
							</RptEnabled>
						</ReportControl>
						<DOI name="NamPlt">
							<DAI name="vendor">
								<Val>OT Fabric Demo</Val>
							</DAI>
							<DAI name="swRev">
								<Val>2.4.0</Val>
							</DAI>
							<DAI name="d">
								<Val>Measurements</Val>
							</DAI>
							<DAI name="configRev">
								<Val>AA1-2026-11-03</Val>
							</DAI>
						</DOI>
					</LN0>
					<LN lnClass="MMXU" inst="1" lnType="MMXU_A" desc="Feeder measurement">
						<DOI name="TotW">
							<DAI name="db">
								<Val>2000</Val>
							</DAI>
						</DOI>
						<DOI name="TotVAr">
							<DAI name="db">
								<Val>2000</Val>
							</DAI>
						</DOI>
						<DOI name="Hz">
							<DAI name="db">
								<Val>100</Val>
							</DAI>
						</DOI>
					</LN>
					<LN lnClass="MSQI" inst="1" lnType="MSQI_A" desc="Sequence currents"/>
					<LN lnClass="MMTR" inst="1" lnType="MMTR_A" desc="Feeder energy meter"/>
				</LDevice>
			</Server>
		</AccessPoint>
	</IED>
	<IED name="AA1D1Q03A1" desc="Feeder 2 protection and control (Hillcrest)" type="FPC-600" manufacturer="OT Fabric Demo" configVersion="1.7" originalSclVersion="2003" originalSclRevision="A">
		<Services>
			<DynAssociation max="4"/>
			<SettingGroups><SGEdit/><ConfSG/></SettingGroups>
			<GetDirectory/>
			<GetDataObjectDefinition/>
			<DataObjectDirectory/>
			<GetDataSetValue/>
			<DataSetDirectory/>
			<ConfDataSet max="12" maxAttributes="64"/>
			<ReadWrite/>
			<ConfReportControl max="12"/>
			<GetCBValues/>
			<ReportSettings rptID="Dyn" optFields="Dyn" bufTime="Dyn" trgOps="Dyn" intgPd="Dyn"/>
			<GSESettings appID="Conf" dataLabel="Fix"/>
			<GOOSE max="4"/>
			<ConfLogControl max="2"/>
			<FileHandling/>
			<ConfLNs fixPrefix="true" fixLnInst="true"/>
		</Services>
		<AccessPoint name="S1" desc="Station bus">
			<Server>
				<Authentication none="true"/>
				<LDevice inst="LD0" desc="Device">
					<LN0 lnClass="LLN0" inst="" lnType="LLN0_E1">
						<DOI name="NamPlt">
							<DAI name="vendor">
								<Val>OT Fabric Demo</Val>
							</DAI>
							<DAI name="swRev">
								<Val>2.4.0</Val>
							</DAI>
							<DAI name="d">
								<Val>Feeder 2 protection and control</Val>
							</DAI>
							<DAI name="configRev">
								<Val>AA1-2026-11-03</Val>
							</DAI>
						</DOI>
					</LN0>
					<LN lnClass="LPHD" inst="1" lnType="LPHD_A">
						<DOI name="PhyNam">
							<DAI name="vendor">
								<Val>OT Fabric Demo</Val>
							</DAI>
							<DAI name="hwRev">
								<Val>C</Val>
							</DAI>
							<DAI name="swRev">
								<Val>2.4.0</Val>
							</DAI>
							<DAI name="serNum">
								<Val>FPC-600-264412</Val>
							</DAI>
							<DAI name="model">
								<Val>FPC-600</Val>
							</DAI>
							<DAI name="location">
								<Val>AA1 Northfield, relay room</Val>
							</DAI>
						</DOI>
					</LN>
					<LN lnClass="GGIO" inst="1" lnType="GGIO_IND3" desc="Breaker supervision inputs"/>
				</LDevice>
				<LDevice inst="PROT" desc="Protection">
					<LN0 lnClass="LLN0" inst="" lnType="LLN0_E1">
						<DataSet name="dsTrip" desc="Protection starts and trips">
							<FCDA ldInst="PROT" lnClass="PTOC" lnInst="1" doName="Str" fc="ST"/>
							<FCDA ldInst="PROT" lnClass="PTOC" lnInst="1" doName="Op" fc="ST"/>
							<FCDA ldInst="PROT" lnClass="PTOC" lnInst="2" doName="Str" fc="ST"/>
							<FCDA ldInst="PROT" lnClass="PTOC" lnInst="2" doName="Op" fc="ST"/>
							<FCDA ldInst="PROT" lnClass="PTRC" lnInst="1" doName="Tr" fc="ST"/>
						</DataSet>
						<DataSet name="dsGoTrip" desc="Trip for GOOSE">
							<FCDA ldInst="PROT" lnClass="PTRC" lnInst="1" doName="Tr" daName="general" fc="ST"/>
							<FCDA ldInst="PROT" lnClass="PTRC" lnInst="1" doName="Tr" daName="q" fc="ST"/>
						</DataSet>
						<ReportControl name="brcbTrip" desc="Protection events to the station HMI" datSet="dsTrip" rptID="AA1D1Q03A1PROT/LLN0.brcbTrip" confRev="1" buffered="true" bufTime="50" indexed="false">
							<TrgOps dchg="true" qchg="true" gi="true"/>
							<OptFields seqNum="true" dataSet="true" reasonCode="true" dataRef="true" entryID="true" configRef="true" bufOvfl="false"/>
							<RptEnabled max="2">
								<ClientLN iedName="AA1KA1" ldInst="LD0" lnClass="IHMI" lnInst="1" apRef="S1"/>
							</RptEnabled>
						</ReportControl>
						<DOI name="NamPlt">
							<DAI name="vendor">
								<Val>OT Fabric Demo</Val>
							</DAI>
							<DAI name="swRev">
								<Val>2.4.0</Val>
							</DAI>
							<DAI name="d">
								<Val>Protection</Val>
							</DAI>
							<DAI name="configRev">
								<Val>AA1-2026-11-03</Val>
							</DAI>
						</DOI>
						<GSEControl name="gcbTrip" desc="Trip to busbar and transformer protection" datSet="dsGoTrip" confRev="1" appID="AA1D1Q03A1_Trip" type="GOOSE"/>
						<SettingControl numOfSGs="2" actSG="1"/>
					</LN0>
					<LN lnClass="PTOC" inst="1" lnType="PTOC_A" desc="Overcurrent I&gt; (51)">
						<DOI name="StrVal">
							<SDI name="setMag">
								<DAI name="f">
									<Val sGroup="1">600</Val>
									<Val sGroup="2">800</Val>
								</DAI>
							</SDI>
							<SDI name="minVal">
								<DAI name="f">
									<Val>50</Val>
								</DAI>
							</SDI>
							<SDI name="maxVal">
								<DAI name="f">
									<Val>5000</Val>
								</DAI>
							</SDI>
							<SDI name="stepSize">
								<DAI name="f">
									<Val>10</Val>
								</DAI>
							</SDI>
						</DOI>
						<DOI name="OpDlTmms">
							<DAI name="setVal">
								<Val sGroup="1">300</Val>
								<Val sGroup="2">500</Val>
							</DAI>
							<DAI name="minVal">
								<Val>0</Val>
							</DAI>
							<DAI name="maxVal">
								<Val>60000</Val>
							</DAI>
							<DAI name="stepSize">
								<Val>10</Val>
							</DAI>
						</DOI>
					</LN>
					<LN lnClass="PTOC" inst="2" lnType="PTOC_A" desc="Overcurrent I&gt;&gt; (50)">
						<DOI name="StrVal">
							<SDI name="setMag">
								<DAI name="f">
									<Val sGroup="1">4000</Val>
									<Val sGroup="2">5000</Val>
								</DAI>
							</SDI>
							<SDI name="minVal">
								<DAI name="f">
									<Val>100</Val>
								</DAI>
							</SDI>
							<SDI name="maxVal">
								<DAI name="f">
									<Val>20000</Val>
								</DAI>
							</SDI>
							<SDI name="stepSize">
								<DAI name="f">
									<Val>10</Val>
								</DAI>
							</SDI>
						</DOI>
						<DOI name="OpDlTmms">
							<DAI name="setVal">
								<Val sGroup="1">50</Val>
								<Val sGroup="2">50</Val>
							</DAI>
							<DAI name="minVal">
								<Val>0</Val>
							</DAI>
							<DAI name="maxVal">
								<Val>60000</Val>
							</DAI>
							<DAI name="stepSize">
								<Val>10</Val>
							</DAI>
						</DOI>
					</LN>
					<LN lnClass="PTRC" inst="1" lnType="PTRC_A" desc="Trip conditioning"/>
				</LDevice>
				<LDevice inst="CTRL" desc="Control">
					<LN0 lnClass="LLN0" inst="" lnType="LLN0_E1">
						<DataSet name="dsPos" desc="Switch positions and interlocking">
							<FCDA ldInst="CTRL" lnClass="CSWI" lnInst="1" doName="Pos" fc="ST"/>
							<FCDA ldInst="CTRL" lnClass="CSWI" lnInst="2" doName="Pos" fc="ST"/>
							<FCDA ldInst="CTRL" lnClass="XCBR" lnInst="1" doName="Pos" fc="ST"/>
							<FCDA ldInst="CTRL" lnClass="XSWI" lnInst="1" doName="Pos" fc="ST"/>
							<FCDA ldInst="CTRL" lnClass="XCBR" lnInst="1" doName="OpCnt" fc="ST"/>
							<FCDA ldInst="CTRL" lnClass="CILO" lnInst="2" doName="EnaOpn" daName="stVal" fc="ST"/>
							<FCDA ldInst="CTRL" lnClass="CILO" lnInst="2" doName="EnaCls" daName="stVal" fc="ST"/>
						</DataSet>
						<DataSet name="dsGoPos" desc="Breaker position for GOOSE">
							<FCDA ldInst="CTRL" lnClass="XCBR" lnInst="1" doName="Pos" daName="stVal" fc="ST"/>
							<FCDA ldInst="CTRL" lnClass="XCBR" lnInst="1" doName="Pos" daName="q" fc="ST"/>
							<FCDA ldInst="CTRL" lnClass="XCBR" lnInst="1" doName="Pos" daName="t" fc="ST"/>
						</DataSet>
						<ReportControl name="brcbPos" desc="Switch positions to the station HMI" datSet="dsPos" rptID="AA1D1Q03A1CTRL/LLN0.brcbPos" confRev="1" buffered="true" bufTime="50" indexed="false">
							<TrgOps dchg="true" qchg="true" gi="true"/>
							<OptFields seqNum="true" dataSet="true" reasonCode="true" entryID="true" configRef="true" bufOvfl="false"/>
							<RptEnabled max="2">
								<ClientLN iedName="AA1KA1" ldInst="LD0" lnClass="IHMI" lnInst="1" apRef="S1"/>
							</RptEnabled>
						</ReportControl>
						<LogControl name="lcbEvents" desc="Switching events" datSet="dsPos" logName="EventLog" logEna="true" reasonCode="true">
							<TrgOps dchg="true" qchg="true"/>
						</LogControl>
						<DOI name="NamPlt">
							<DAI name="vendor">
								<Val>OT Fabric Demo</Val>
							</DAI>
							<DAI name="swRev">
								<Val>2.4.0</Val>
							</DAI>
							<DAI name="d">
								<Val>Control</Val>
							</DAI>
							<DAI name="configRev">
								<Val>AA1-2026-11-03</Val>
							</DAI>
						</DOI>
						<Log name="EventLog"/>
						<GSEControl name="gcbPos" desc="Breaker position to the busbar protection" datSet="dsGoPos" confRev="1" appID="AA1D1Q03A1_Pos" type="GOOSE"/>
					</LN0>
					<LN lnClass="CSWI" inst="1" lnType="CSWI_SBOE" desc="Circuit breaker QA1 control"/>
					<LN lnClass="CSWI" inst="2" lnType="CSWI_SBOE" desc="Disconnector QB1 control"/>
					<LN lnClass="XCBR" inst="1" lnType="XCBR_A" desc="Circuit breaker QA1"/>
					<LN lnClass="XSWI" inst="1" lnType="XSWI_A" desc="Disconnector QB1">
						<DOI name="SwTyp">
							<DAI name="stVal">
								<Val>Disconnector</Val>
							</DAI>
						</DOI>
					</LN>
					<LN lnClass="CILO" inst="1" lnType="CILO_A" desc="Interlocking QA1"/>
					<LN lnClass="CILO" inst="2" lnType="CILO_A" desc="Interlocking QB1 (open QA1 first)"/>
				</LDevice>
				<LDevice inst="MEAS" desc="Measurements">
					<LN0 lnClass="LLN0" inst="" lnType="LLN0_E1">
						<DataSet name="dsMeas" desc="Feeder measurements">
							<FCDA ldInst="MEAS" lnClass="MMXU" lnInst="1" doName="TotW" fc="MX"/>
							<FCDA ldInst="MEAS" lnClass="MMXU" lnInst="1" doName="TotVAr" fc="MX"/>
							<FCDA ldInst="MEAS" lnClass="MMXU" lnInst="1" doName="Hz" fc="MX"/>
							<FCDA ldInst="MEAS" lnClass="MMXU" lnInst="1" doName="A" fc="MX"/>
							<FCDA ldInst="MEAS" lnClass="MMXU" lnInst="1" doName="PhV" fc="MX"/>
							<FCDA ldInst="MEAS" lnClass="MSQI" lnInst="1" doName="SeqA" fc="MX"/>
						</DataSet>
						<ReportControl name="urcbMeas" desc="Measurements to the station HMI" datSet="dsMeas" intgPd="10000" rptID="AA1D1Q03A1MEAS/LLN0.urcbMeas" confRev="1" buffered="false" bufTime="100" indexed="false">
							<TrgOps dchg="true" qchg="true" period="true" gi="true"/>
							<OptFields seqNum="true" dataSet="true" reasonCode="true" configRef="true" bufOvfl="false"/>
							<RptEnabled max="2">
								<ClientLN iedName="AA1KA1" ldInst="LD0" lnClass="IHMI" lnInst="1" apRef="S1"/>
							</RptEnabled>
						</ReportControl>
						<DOI name="NamPlt">
							<DAI name="vendor">
								<Val>OT Fabric Demo</Val>
							</DAI>
							<DAI name="swRev">
								<Val>2.4.0</Val>
							</DAI>
							<DAI name="d">
								<Val>Measurements</Val>
							</DAI>
							<DAI name="configRev">
								<Val>AA1-2026-11-03</Val>
							</DAI>
						</DOI>
					</LN0>
					<LN lnClass="MMXU" inst="1" lnType="MMXU_A" desc="Feeder measurement">
						<DOI name="TotW">
							<DAI name="db">
								<Val>2000</Val>
							</DAI>
						</DOI>
						<DOI name="TotVAr">
							<DAI name="db">
								<Val>2000</Val>
							</DAI>
						</DOI>
						<DOI name="Hz">
							<DAI name="db">
								<Val>100</Val>
							</DAI>
						</DOI>
					</LN>
					<LN lnClass="MSQI" inst="1" lnType="MSQI_A" desc="Sequence currents"/>
					<LN lnClass="MMTR" inst="1" lnType="MMTR_A" desc="Feeder energy meter"/>
				</LDevice>
			</Server>
		</AccessPoint>
	</IED>
	<IED name="AA1D1W1A1" desc="20 kV busbar and breaker failure protection" type="BPC-670" manufacturer="OT Fabric Demo" configVersion="1.4" originalSclVersion="2007" originalSclRevision="B" originalSclRelease="4">
		<Services>
			<DynAssociation max="4"/>
			<SettingGroups><SGEdit/><ConfSG/></SettingGroups>
			<GetDirectory/>
			<GetDataObjectDefinition/>
			<DataObjectDirectory/>
			<GetDataSetValue/>
			<DataSetDirectory/>
			<ConfDataSet max="12" maxAttributes="64"/>
			<ReadWrite/>
			<ConfReportControl max="12"/>
			<GetCBValues/>
			<ReportSettings rptID="Dyn" optFields="Dyn" bufTime="Dyn" trgOps="Dyn" intgPd="Dyn"/>
			<GSESettings appID="Conf" dataLabel="Fix"/>
			<GOOSE max="4"/>
			<ConfLogControl max="2"/>
			<FileHandling/>
			<ConfLNs fixPrefix="true" fixLnInst="true"/>
		</Services>
		<AccessPoint name="S1" desc="Station bus">
			<Server>
				<Authentication none="true"/>
				<LDevice inst="LD0" desc="Device">
					<LN0 lnClass="LLN0" inst="" lnType="LLN0_A">
						<DOI name="NamPlt">
							<DAI name="vendor">
								<Val>OT Fabric Demo</Val>
							</DAI>
							<DAI name="swRev">
								<Val>2.4.0</Val>
							</DAI>
							<DAI name="d">
								<Val>Busbar protection</Val>
							</DAI>
							<DAI name="configRev">
								<Val>AA1-2026-11-03</Val>
							</DAI>
						</DOI>
					</LN0>
					<LN lnClass="LPHD" inst="1" lnType="LPHD_A">
						<DOI name="PhyNam">
							<DAI name="vendor">
								<Val>OT Fabric Demo</Val>
							</DAI>
							<DAI name="hwRev">
								<Val>C</Val>
							</DAI>
							<DAI name="swRev">
								<Val>2.4.0</Val>
							</DAI>
							<DAI name="serNum">
								<Val>BPC670-260077</Val>
							</DAI>
							<DAI name="model">
								<Val>BPC-670</Val>
							</DAI>
							<DAI name="location">
								<Val>AA1 Northfield, relay room</Val>
							</DAI>
						</DOI>
					</LN>
				</LDevice>
				<LDevice inst="PROT" desc="Protection">
					<LN0 lnClass="LLN0" inst="" lnType="LLN0_A">
						<DataSet name="dsGoBF" desc="Breaker failure for GOOSE">
							<FCDA ldInst="PROT" lnClass="RBRF" lnInst="1" doName="OpEx" daName="general" fc="ST"/>
							<FCDA ldInst="PROT" lnClass="RBRF" lnInst="1" doName="OpEx" daName="q" fc="ST"/>
							<FCDA ldInst="PROT" lnClass="RBRF" lnInst="2" doName="OpEx" daName="general" fc="ST"/>
							<FCDA ldInst="PROT" lnClass="RBRF" lnInst="2" doName="OpEx" daName="q" fc="ST"/>
						</DataSet>
						<DataSet name="dsTrip" desc="Busbar protection and breaker failure events">
							<FCDA ldInst="PROT" lnClass="PDIF" lnInst="1" doName="Op" fc="ST"/>
							<FCDA ldInst="PROT" lnClass="RBRF" lnInst="1" doName="OpEx" fc="ST"/>
							<FCDA ldInst="PROT" lnClass="RBRF" lnInst="1" doName="OpIn" fc="ST"/>
							<FCDA ldInst="PROT" lnClass="RBRF" lnInst="2" doName="OpEx" fc="ST"/>
							<FCDA ldInst="PROT" lnClass="RBRF" lnInst="2" doName="OpIn" fc="ST"/>
							<FCDA ldInst="PROT" lnClass="PTRC" lnInst="1" doName="Tr" fc="ST"/>
						</DataSet>
						<ReportControl name="brcbTrip" desc="Protection events to the station HMI" datSet="dsTrip" rptID="AA1D1W1A1PROT/LLN0.brcbTrip" confRev="1" buffered="true" bufTime="50" indexed="false">
							<TrgOps dchg="true" qchg="true" gi="true"/>
							<OptFields seqNum="true" dataSet="true" reasonCode="true" dataRef="true" entryID="true" configRef="true" bufOvfl="false"/>
							<RptEnabled max="2">
								<ClientLN iedName="AA1KA1" ldInst="LD0" lnClass="IHMI" lnInst="1" apRef="S1"/>
							</RptEnabled>
						</ReportControl>
						<DOI name="NamPlt">
							<DAI name="vendor">
								<Val>OT Fabric Demo</Val>
							</DAI>
							<DAI name="swRev">
								<Val>2.4.0</Val>
							</DAI>
							<DAI name="d">
								<Val>Protection</Val>
							</DAI>
							<DAI name="configRev">
								<Val>AA1-2026-11-03</Val>
							</DAI>
						</DOI>
						<GSEControl name="gcbBF" desc="Breaker failure to the transformer protection" datSet="dsGoBF" confRev="1" appID="AA1D1W1A1_BF" type="GOOSE"/>
					</LN0>
					<LN lnClass="PDIF" inst="1" lnType="PDIF_A" desc="Busbar differential (87B)">
						<DOI name="LoSet">
							<SDI name="setMag">
								<DAI name="f">
									<Val sGroup="1">1200</Val>
								</DAI>
							</SDI>
						</DOI>
						<Inputs>
							<ExtRef desc="Feeder 1 current phase A" iedName="AA1D1Q02MU1" ldInst="MU01" prefix="I" lnClass="TCTR" lnInst="1" doName="AmpSv" daName="instMag.i" intAddr="IF1L1" serviceType="SMV" srcLDInst="MU01" srcLNClass="LLN0" srcCBName="MSVCB01"/>
							<ExtRef desc="Feeder 1 current phase B" iedName="AA1D1Q02MU1" ldInst="MU01" prefix="I" lnClass="TCTR" lnInst="2" doName="AmpSv" daName="instMag.i" intAddr="IF1L2" serviceType="SMV" srcLDInst="MU01" srcLNClass="LLN0" srcCBName="MSVCB01"/>
							<ExtRef desc="Feeder 1 current phase C" iedName="AA1D1Q02MU1" ldInst="MU01" prefix="I" lnClass="TCTR" lnInst="3" doName="AmpSv" daName="instMag.i" intAddr="IF1L3" serviceType="SMV" srcLDInst="MU01" srcLNClass="LLN0" srcCBName="MSVCB01"/>
						</Inputs>
					</LN>
					<LN lnClass="RBRF" inst="1" lnType="RBRF_A" desc="Breaker failure feeder 1 (50BF)">
						<DOI name="FailTmms">
							<DAI name="setVal">
								<Val sGroup="1">150</Val>
							</DAI>
						</DOI>
						<Inputs>
							<ExtRef desc="Feeder 1 trip starts breaker failure" iedName="AA1D1Q02A1" ldInst="PROT" lnClass="PTRC" lnInst="1" doName="Tr" daName="general" intAddr="StrBF1" serviceType="GOOSE" srcLDInst="PROT" srcLNClass="LLN0" srcCBName="gcbTrip"/>
						</Inputs>
					</LN>
					<LN lnClass="RBRF" inst="2" lnType="RBRF_A" desc="Breaker failure feeder 2 (50BF)">
						<DOI name="FailTmms">
							<DAI name="setVal">
								<Val sGroup="1">150</Val>
							</DAI>
						</DOI>
						<Inputs>
							<ExtRef desc="Feeder 2 trip starts breaker failure" iedName="AA1D1Q03A1" ldInst="PROT" lnClass="PTRC" lnInst="1" doName="Tr" daName="general" intAddr="StrBF2" serviceType="GOOSE" srcLDInst="PROT" srcLNClass="LLN0" srcCBName="gcbTrip"/>
						</Inputs>
					</LN>
					<LN lnClass="PTRC" inst="1" lnType="PTRC_A" desc="Trip conditioning"/>
				</LDevice>
			</Server>
		</AccessPoint>
	</IED>
	<IED name="AA1D1Q02MU1" desc="Merging unit feeder 1 (UCA 9-2LE)" type="MU-920" manufacturer="OT Fabric Demo" configVersion="1.1" originalSclVersion="2007" originalSclRevision="B" originalSclRelease="4">
		<Services>
			<GetDirectory/>
			<GetDataObjectDefinition/>
			<DataObjectDirectory/>
			<GetDataSetValue/>
			<DataSetDirectory/>
			<ConfDataSet max="12" maxAttributes="64"/>
			<ReadWrite/>
			<GetCBValues/>
			<GSESettings appID="Conf" dataLabel="Fix"/>
			<SMVSettings svID="Conf" optFields="Fix" smpRate="Fix"><SmpRate>80</SmpRate></SMVSettings>
			<GOOSE max="1"/>
			<SMVsc max="1"/>
		</Services>
		<AccessPoint name="S1" desc="Station bus">
			<Server>
				<Authentication none="true"/>
				<LDevice inst="MU01" desc="Merging unit">
					<LN0 lnClass="LLN0" inst="" lnType="LLN0_A">
						<DataSet name="PhsMeas1" desc="UCA 9-2LE: 4 currents, 4 voltages">
							<FCDA ldInst="MU01" prefix="I" lnClass="TCTR" lnInst="1" doName="AmpSv" daName="instMag.i" fc="MX"/>
							<FCDA ldInst="MU01" prefix="I" lnClass="TCTR" lnInst="1" doName="AmpSv" daName="q" fc="MX"/>
							<FCDA ldInst="MU01" prefix="I" lnClass="TCTR" lnInst="2" doName="AmpSv" daName="instMag.i" fc="MX"/>
							<FCDA ldInst="MU01" prefix="I" lnClass="TCTR" lnInst="2" doName="AmpSv" daName="q" fc="MX"/>
							<FCDA ldInst="MU01" prefix="I" lnClass="TCTR" lnInst="3" doName="AmpSv" daName="instMag.i" fc="MX"/>
							<FCDA ldInst="MU01" prefix="I" lnClass="TCTR" lnInst="3" doName="AmpSv" daName="q" fc="MX"/>
							<FCDA ldInst="MU01" prefix="I" lnClass="TCTR" lnInst="4" doName="AmpSv" daName="instMag.i" fc="MX"/>
							<FCDA ldInst="MU01" prefix="I" lnClass="TCTR" lnInst="4" doName="AmpSv" daName="q" fc="MX"/>
							<FCDA ldInst="MU01" prefix="U" lnClass="TVTR" lnInst="1" doName="VolSv" daName="instMag.i" fc="MX"/>
							<FCDA ldInst="MU01" prefix="U" lnClass="TVTR" lnInst="1" doName="VolSv" daName="q" fc="MX"/>
							<FCDA ldInst="MU01" prefix="U" lnClass="TVTR" lnInst="2" doName="VolSv" daName="instMag.i" fc="MX"/>
							<FCDA ldInst="MU01" prefix="U" lnClass="TVTR" lnInst="2" doName="VolSv" daName="q" fc="MX"/>
							<FCDA ldInst="MU01" prefix="U" lnClass="TVTR" lnInst="3" doName="VolSv" daName="instMag.i" fc="MX"/>
							<FCDA ldInst="MU01" prefix="U" lnClass="TVTR" lnInst="3" doName="VolSv" daName="q" fc="MX"/>
							<FCDA ldInst="MU01" prefix="U" lnClass="TVTR" lnInst="4" doName="VolSv" daName="instMag.i" fc="MX"/>
							<FCDA ldInst="MU01" prefix="U" lnClass="TVTR" lnInst="4" doName="VolSv" daName="q" fc="MX"/>
						</DataSet>
						<DataSet name="dsStatus" desc="Merging unit status for GOOSE">
							<FCDA ldInst="MU01" lnClass="LPHD" lnInst="1" doName="PhyHealth" daName="stVal" fc="ST"/>
							<FCDA ldInst="MU01" lnClass="LPHD" lnInst="1" doName="PhyHealth" daName="q" fc="ST"/>
							<FCDA ldInst="MU01" lnClass="LLN0" doName="Beh" daName="stVal" fc="ST"/>
						</DataSet>
						<DOI name="NamPlt">
							<DAI name="vendor">
								<Val>OT Fabric Demo</Val>
							</DAI>
							<DAI name="swRev">
								<Val>2.4.0</Val>
							</DAI>
							<DAI name="d">
								<Val>Merging unit</Val>
							</DAI>
							<DAI name="configRev">
								<Val>AA1-2026-11-03</Val>
							</DAI>
						</DOI>
						<GSEControl name="gcbStatus" desc="Merging unit health" datSet="dsStatus" confRev="1" appID="AA1D1Q02MU1_Status" type="GOOSE"/>
						<SampledValueControl name="MSVCB01" desc="Feeder 1 currents and voltages" datSet="PhsMeas1" confRev="1" smvID="AA1D1Q02MU1MU01" smpRate="80" nofASDU="1" multicast="true" smpMod="SmpPerPeriod">
							<SmvOpts sampleSynchronized="true"/>
						</SampledValueControl>
					</LN0>
					<LN lnClass="LPHD" inst="1" lnType="LPHD_A">
						<DOI name="PhyNam">
							<DAI name="vendor">
								<Val>OT Fabric Demo</Val>
							</DAI>
							<DAI name="hwRev">
								<Val>C</Val>
							</DAI>
							<DAI name="swRev">
								<Val>2.4.0</Val>
							</DAI>
							<DAI name="serNum">
								<Val>MU920-260142</Val>
							</DAI>
							<DAI name="model">
								<Val>MU-920</Val>
							</DAI>
							<DAI name="location">
								<Val>AA1 Northfield, relay room</Val>
							</DAI>
						</DOI>
					</LN>
					<LN prefix="I" lnClass="TCTR" inst="1" lnType="TCTR_LE" desc="Current phase A">
						<DOI name="HzRtg">
							<SDI name="setMag">
								<DAI name="f">
									<Val>50</Val>
								</DAI>
							</SDI>
						</DOI>
						<DOI name="ARtg">
							<SDI name="setMag">
								<DAI name="f">
									<Val>600</Val>
								</DAI>
							</SDI>
						</DOI>
					</LN>
					<LN prefix="I" lnClass="TCTR" inst="2" lnType="TCTR_LE" desc="Current phase B">
						<DOI name="HzRtg">
							<SDI name="setMag">
								<DAI name="f">
									<Val>50</Val>
								</DAI>
							</SDI>
						</DOI>
						<DOI name="ARtg">
							<SDI name="setMag">
								<DAI name="f">
									<Val>600</Val>
								</DAI>
							</SDI>
						</DOI>
					</LN>
					<LN prefix="I" lnClass="TCTR" inst="3" lnType="TCTR_LE" desc="Current phase C">
						<DOI name="HzRtg">
							<SDI name="setMag">
								<DAI name="f">
									<Val>50</Val>
								</DAI>
							</SDI>
						</DOI>
						<DOI name="ARtg">
							<SDI name="setMag">
								<DAI name="f">
									<Val>600</Val>
								</DAI>
							</SDI>
						</DOI>
					</LN>
					<LN prefix="I" lnClass="TCTR" inst="4" lnType="TCTR_LE" desc="Current phase N">
						<DOI name="HzRtg">
							<SDI name="setMag">
								<DAI name="f">
									<Val>50</Val>
								</DAI>
							</SDI>
						</DOI>
						<DOI name="ARtg">
							<SDI name="setMag">
								<DAI name="f">
									<Val>600</Val>
								</DAI>
							</SDI>
						</DOI>
					</LN>
					<LN prefix="U" lnClass="TVTR" inst="1" lnType="TVTR_LE" desc="Voltage phase A">
						<DOI name="HzRtg">
							<SDI name="setMag">
								<DAI name="f">
									<Val>50</Val>
								</DAI>
							</SDI>
						</DOI>
						<DOI name="VRtg">
							<SDI name="setMag">
								<DAI name="f">
									<Val>11547</Val>
								</DAI>
							</SDI>
						</DOI>
					</LN>
					<LN prefix="U" lnClass="TVTR" inst="2" lnType="TVTR_LE" desc="Voltage phase B">
						<DOI name="HzRtg">
							<SDI name="setMag">
								<DAI name="f">
									<Val>50</Val>
								</DAI>
							</SDI>
						</DOI>
						<DOI name="VRtg">
							<SDI name="setMag">
								<DAI name="f">
									<Val>11547</Val>
								</DAI>
							</SDI>
						</DOI>
					</LN>
					<LN prefix="U" lnClass="TVTR" inst="3" lnType="TVTR_LE" desc="Voltage phase C">
						<DOI name="HzRtg">
							<SDI name="setMag">
								<DAI name="f">
									<Val>50</Val>
								</DAI>
							</SDI>
						</DOI>
						<DOI name="VRtg">
							<SDI name="setMag">
								<DAI name="f">
									<Val>11547</Val>
								</DAI>
							</SDI>
						</DOI>
					</LN>
					<LN prefix="U" lnClass="TVTR" inst="4" lnType="TVTR_LE" desc="Voltage phase N">
						<DOI name="HzRtg">
							<SDI name="setMag">
								<DAI name="f">
									<Val>50</Val>
								</DAI>
							</SDI>
						</DOI>
						<DOI name="VRtg">
							<SDI name="setMag">
								<DAI name="f">
									<Val>11547</Val>
								</DAI>
							</SDI>
						</DOI>
					</LN>
				</LDevice>
			</Server>
		</AccessPoint>
	</IED>
	<DataTypeTemplates>
		<LNodeType id="LLN0_A" lnClass="LLN0" desc="Logical device root, Edition 2">
			<DO name="Mod" type="ENC_Mod"/>
			<DO name="Beh" type="ENS_Beh"/>
			<DO name="Health" type="ENS_Health"/>
			<DO name="NamPlt" type="LPL_LD"/>
		</LNodeType>
		<LNodeType id="LLN0_E1" lnClass="LLN0" desc="Logical device root, Edition 1">
			<DO name="Mod" type="ENC_Mod"/>
			<DO name="Beh" type="ENS_Beh"/>
			<DO name="Health" type="ENS_Health"/>
			<DO name="NamPlt" type="LPL_E1"/>
		</LNodeType>
		<LNodeType id="LPHD_A" lnClass="LPHD">
			<DO name="PhyNam" type="DPL_A"/>
			<DO name="PhyHealth" type="ENS_Health"/>
			<DO name="Proxy" type="SPS_A"/>
		</LNodeType>
		<LNodeType id="GGIO_IND3" lnClass="GGIO" desc="Three binary inputs">
			<DO name="Beh" type="ENS_Beh"/>
			<DO name="Ind1" type="SPS_A" desc="SF6 pressure low"/>
			<DO name="Ind2" type="SPS_A" desc="Spring charged"/>
			<DO name="Ind3" type="SPS_A" desc="Trip circuit supervision"/>
		</LNodeType>
		<LNodeType id="PTOC_A" lnClass="PTOC">
			<DO name="Beh" type="ENS_Beh"/>
			<DO name="Str" type="ACD_A"/>
			<DO name="Op" type="ACT_A"/>
			<DO name="StrVal" type="ASG_SG"/>
			<DO name="OpDlTmms" type="ING_SG"/>
		</LNodeType>
		<LNodeType id="PTRC_A" lnClass="PTRC">
			<DO name="Beh" type="ENS_Beh"/>
			<DO name="Str" type="ACD_A"/>
			<DO name="Op" type="ACT_A"/>
			<DO name="Tr" type="ACT_G"/>
		</LNodeType>
		<LNodeType id="PDIF_A" lnClass="PDIF">
			<DO name="Beh" type="ENS_Beh"/>
			<DO name="Str" type="ACD_A"/>
			<DO name="Op" type="ACT_A"/>
			<DO name="LoSet" type="ASG_SG"/>
		</LNodeType>
		<LNodeType id="RBRF_A" lnClass="RBRF">
			<DO name="Beh" type="ENS_Beh"/>
			<DO name="Str" type="ACD_A"/>
			<DO name="OpEx" type="ACT_G"/>
			<DO name="OpIn" type="ACT_G"/>
			<DO name="FailTmms" type="ING_SG"/>
		</LNodeType>
		<LNodeType id="CSWI_SBOE" lnClass="CSWI" desc="Switch controller, SBO with enhanced security">
			<DO name="Beh" type="ENS_Beh"/>
			<DO name="Pos" type="DPC_SBOE"/>
		</LNodeType>
		<LNodeType id="XCBR_A" lnClass="XCBR">
			<DO name="Beh" type="ENS_Beh"/>
			<DO name="Loc" type="SPS_A"/>
			<DO name="OpCnt" type="INS_A"/>
			<DO name="Pos" type="DPC_ST"/>
			<DO name="BlkOpn" type="SPC_ST"/>
			<DO name="BlkCls" type="SPC_ST"/>
		</LNodeType>
		<LNodeType id="XSWI_A" lnClass="XSWI">
			<DO name="Beh" type="ENS_Beh"/>
			<DO name="Loc" type="SPS_A"/>
			<DO name="OpCnt" type="INS_A"/>
			<DO name="Pos" type="DPC_ST"/>
			<DO name="BlkOpn" type="SPC_ST"/>
			<DO name="BlkCls" type="SPC_ST"/>
			<DO name="SwTyp" type="ENS_SwTyp"/>
		</LNodeType>
		<LNodeType id="CILO_A" lnClass="CILO">
			<DO name="Beh" type="ENS_Beh"/>
			<DO name="EnaOpn" type="SPS_A"/>
			<DO name="EnaCls" type="SPS_A"/>
		</LNodeType>
		<LNodeType id="MMXU_A" lnClass="MMXU" desc="Three-phase measurement: every quantity the Explorer&#39;s search knows">
			<DO name="Beh" type="ENS_Beh"/>
			<DO name="TotW" type="MV_A"/>
			<DO name="TotVAr" type="MV_A"/>
			<DO name="TotVA" type="MV_A"/>
			<DO name="TotPF" type="MV_A"/>
			<DO name="Hz" type="MV_A"/>
			<DO name="PPV" type="DEL_A"/>
			<DO name="PNV" type="WYE_A"/>
			<DO name="PhV" type="WYE_A"/>
			<DO name="A" type="WYE_A"/>
			<DO name="W" type="WYE_A"/>
			<DO name="VAr" type="WYE_A"/>
			<DO name="VA" type="WYE_A"/>
			<DO name="PF" type="WYE_A"/>
			<DO name="Z" type="WYE_A"/>
		</LNodeType>
		<LNodeType id="MMXN_A" lnClass="MMXN" desc="Single-phase measurement">
			<DO name="Beh" type="ENS_Beh"/>
			<DO name="Amp" type="MV_A"/>
			<DO name="Vol" type="MV_A"/>
			<DO name="Watt" type="MV_A"/>
			<DO name="VolAmpr" type="MV_A"/>
			<DO name="VolAmp" type="MV_A"/>
			<DO name="PwrFact" type="MV_A"/>
			<DO name="Hz" type="MV_A"/>
		</LNodeType>
		<LNodeType id="MMTR_A" lnClass="MMTR" desc="Energy metering">
			<DO name="Beh" type="ENS_Beh"/>
			<DO name="TotWh" type="BCR_A"/>
			<DO name="TotVArh" type="BCR_A"/>
			<DO name="TotVAh" type="BCR_A"/>
			<DO name="SupWh" type="BCR_A"/>
			<DO name="SupVArh" type="BCR_A"/>
			<DO name="DmdWh" type="BCR_A"/>
			<DO name="DmdVArh" type="BCR_A"/>
		</LNodeType>
		<LNodeType id="MHAI_A" lnClass="MHAI" desc="Harmonics">
			<DO name="Beh" type="ENS_Beh"/>
			<DO name="Hz" type="MV_A"/>
		</LNodeType>
		<LNodeType id="MSQI_A" lnClass="MSQI">
			<DO name="Beh" type="ENS_Beh"/>
			<DO name="SeqA" type="SEQ_A"/>
		</LNodeType>
		<LNodeType id="YPTR_A" lnClass="YPTR">
			<DO name="Beh" type="ENS_Beh"/>
			<DO name="OpOvA" type="SPS_A"/>
			<DO name="LodFact" type="MV_A"/>
		</LNodeType>
		<LNodeType id="YLTC_CV" lnClass="YLTC" desc="Tap changer with Corvane extension data objects">
			<DO name="Beh" type="ENS_Beh"/>
			<DO name="NamPlt" type="LPL_LN"/>
			<DO name="EndPosR" type="SPS_A"/>
			<DO name="EndPosL" type="SPS_A"/>
			<DO name="OpCnt" type="INS_A"/>
			<DO name="TapPos" type="ISC_A"/>
			<DO name="MotDrvTmp" type="MV_VND" desc="Motor drive temperature (vendor extension)"/>
			<DO name="MotDrvA" type="MV_VND" desc="Motor drive current (vendor extension)"/>
		</LNodeType>
		<LNodeType id="ATCC_A" lnClass="ATCC">
			<DO name="Beh" type="ENS_Beh"/>
			<DO name="Auto" type="SPC_DIR"/>
			<DO name="TapPos" type="ISC_A"/>
			<DO name="BndCtr" type="ASG_SG"/>
			<DO name="BndWid" type="ASG_SG"/>
		</LNodeType>
		<LNodeType id="TCTR_LE" lnClass="TCTR" desc="Current transformer, UCA 9-2LE">
			<DO name="Beh" type="ENS_Beh"/>
			<DO name="HzRtg" type="ASG_SP"/>
			<DO name="ARtg" type="ASG_SP"/>
			<DO name="AmpSv" type="SAV_A"/>
		</LNodeType>
		<LNodeType id="TVTR_LE" lnClass="TVTR" desc="Voltage transformer, UCA 9-2LE">
			<DO name="Beh" type="ENS_Beh"/>
			<DO name="HzRtg" type="ASG_SP"/>
			<DO name="VRtg" type="ASG_SP"/>
			<DO name="VolSv" type="SAV_A"/>
		</LNodeType>
		<LNodeType id="IHMI_A" lnClass="IHMI">
			<DO name="Beh" type="ENS_Beh"/>
		</LNodeType>
		<DOType id="ENC_Mod" cdc="ENC">
			<DA name="stVal" fc="ST" bType="Enum" type="BehaviourModeKind" dchg="true"/>
			<DA name="q" fc="ST" bType="Quality" qchg="true"/>
			<DA name="t" fc="ST" bType="Timestamp"/>
			<DA name="ctlModel" fc="CF" bType="Enum" type="CtlModelKind">
				<Val>status-only</Val>
			</DA>
		</DOType>
		<DOType id="ENS_Beh" cdc="ENS">
			<DA name="stVal" fc="ST" bType="Enum" type="BehaviourModeKind" dchg="true"/>
			<DA name="q" fc="ST" bType="Quality" qchg="true"/>
			<DA name="t" fc="ST" bType="Timestamp"/>
		</DOType>
		<DOType id="ENS_Health" cdc="ENS">
			<DA name="stVal" fc="ST" bType="Enum" type="HealthKind" dchg="true"/>
			<DA name="q" fc="ST" bType="Quality" qchg="true"/>
			<DA name="t" fc="ST" bType="Timestamp"/>
		</DOType>
		<DOType id="ENS_SwTyp" cdc="ENS">
			<DA name="stVal" fc="ST" bType="Enum" type="SwitchFunctionKind" dchg="true"/>
			<DA name="q" fc="ST" bType="Quality" qchg="true"/>
			<DA name="t" fc="ST" bType="Timestamp"/>
		</DOType>
		<DOType id="LPL_LD" cdc="LPL">
			<DA name="vendor" fc="DC" bType="VisString255"/>
			<DA name="swRev" fc="DC" bType="VisString255"/>
			<DA name="d" fc="DC" bType="VisString255"/>
			<DA name="configRev" fc="DC" bType="VisString255"/>
			<DA name="ldNs" fc="EX" bType="VisString255">
				<Val>IEC 61850-7-4:2007B</Val>
			</DA>
		</DOType>
		<DOType id="LPL_E1" cdc="LPL">
			<DA name="vendor" fc="DC" bType="VisString255"/>
			<DA name="swRev" fc="DC" bType="VisString255"/>
			<DA name="d" fc="DC" bType="VisString255"/>
			<DA name="configRev" fc="DC" bType="VisString255"/>
		</DOType>
		<DOType id="LPL_LN" cdc="LPL">
			<DA name="vendor" fc="DC" bType="VisString255"/>
			<DA name="swRev" fc="DC" bType="VisString255"/>
			<DA name="d" fc="DC" bType="VisString255"/>
			<DA name="lnNs" fc="EX" bType="VisString255"/>
		</DOType>
		<DOType id="DPL_A" cdc="DPL">
			<DA name="vendor" fc="DC" bType="VisString255"/>
			<DA name="hwRev" fc="DC" bType="VisString255"/>
			<DA name="swRev" fc="DC" bType="VisString255"/>
			<DA name="serNum" fc="DC" bType="VisString255"/>
			<DA name="model" fc="DC" bType="VisString255"/>
			<DA name="location" fc="DC" bType="VisString255"/>
		</DOType>
		<DOType id="SPS_A" cdc="SPS">
			<DA name="stVal" fc="ST" bType="BOOLEAN" dchg="true"/>
			<DA name="q" fc="ST" bType="Quality" qchg="true"/>
			<DA name="t" fc="ST" bType="Timestamp"/>
		</DOType>
		<DOType id="INS_A" cdc="INS">
			<DA name="stVal" fc="ST" bType="INT32" dchg="true"/>
			<DA name="q" fc="ST" bType="Quality" qchg="true"/>
			<DA name="t" fc="ST" bType="Timestamp"/>
		</DOType>
		<DOType id="ACD_A" cdc="ACD">
			<DA name="general" fc="ST" bType="BOOLEAN" dchg="true"/>
			<DA name="dirGeneral" fc="ST" bType="Enum" type="FaultDirectionKind" dchg="true"/>
			<DA name="phsA" fc="ST" bType="BOOLEAN" dchg="true"/>
			<DA name="dirPhsA" fc="ST" bType="Enum" type="PhaseFaultDirectionKind" dchg="true"/>
			<DA name="phsB" fc="ST" bType="BOOLEAN" dchg="true"/>
			<DA name="dirPhsB" fc="ST" bType="Enum" type="PhaseFaultDirectionKind" dchg="true"/>
			<DA name="phsC" fc="ST" bType="BOOLEAN" dchg="true"/>
			<DA name="dirPhsC" fc="ST" bType="Enum" type="PhaseFaultDirectionKind" dchg="true"/>
			<DA name="q" fc="ST" bType="Quality" qchg="true"/>
			<DA name="t" fc="ST" bType="Timestamp"/>
		</DOType>
		<DOType id="ACT_A" cdc="ACT">
			<DA name="general" fc="ST" bType="BOOLEAN" dchg="true"/>
			<DA name="phsA" fc="ST" bType="BOOLEAN" dchg="true"/>
			<DA name="phsB" fc="ST" bType="BOOLEAN" dchg="true"/>
			<DA name="phsC" fc="ST" bType="BOOLEAN" dchg="true"/>
			<DA name="q" fc="ST" bType="Quality" qchg="true"/>
			<DA name="t" fc="ST" bType="Timestamp"/>
		</DOType>
		<DOType id="ACT_G" cdc="ACT">
			<DA name="general" fc="ST" bType="BOOLEAN" dchg="true"/>
			<DA name="q" fc="ST" bType="Quality" qchg="true"/>
			<DA name="t" fc="ST" bType="Timestamp"/>
		</DOType>
		<DOType id="MV_A" cdc="MV">
			<DA name="mag" fc="MX" bType="Struct" type="AnalogueValue" dchg="true"/>
			<DA name="q" fc="MX" bType="Quality" qchg="true"/>
			<DA name="t" fc="MX" bType="Timestamp"/>
			<DA name="db" fc="CF" bType="INT32U"/>
		</DOType>
		<DOType id="MV_VND" cdc="MV">
			<DA name="mag" fc="MX" bType="Struct" type="AnalogueValue" dchg="true"/>
			<DA name="q" fc="MX" bType="Quality" qchg="true"/>
			<DA name="t" fc="MX" bType="Timestamp"/>
			<DA name="dataNs" fc="EX" bType="VisString255">
				<Val>CORVANE 7-4X:2026A</Val>
			</DA>
		</DOType>
		<DOType id="CMV_A" cdc="CMV">
			<DA name="cVal" fc="MX" bType="Struct" type="Vector" dchg="true"/>
			<DA name="q" fc="MX" bType="Quality" qchg="true"/>
			<DA name="t" fc="MX" bType="Timestamp"/>
		</DOType>
		<DOType id="WYE_A" cdc="WYE">
			<SDO name="phsA" type="CMV_A"/>
			<SDO name="phsB" type="CMV_A"/>
			<SDO name="phsC" type="CMV_A"/>
		</DOType>
		<DOType id="DEL_A" cdc="DEL">
			<SDO name="phsAB" type="CMV_A"/>
			<SDO name="phsBC" type="CMV_A"/>
			<SDO name="phsCA" type="CMV_A"/>
		</DOType>
		<DOType id="BCR_A" cdc="BCR">
			<DA name="actVal" fc="ST" bType="INT64" dchg="true"/>
			<DA name="q" fc="ST" bType="Quality" qchg="true"/>
			<DA name="t" fc="ST" bType="Timestamp"/>
			<DA name="pulsQty" fc="CF" bType="FLOAT32">
				<Val>1</Val>
			</DA>
		</DOType>
		<DOType id="SEQ_A" cdc="SEQ">
			<SDO name="c1" type="CMV_A"/>
			<SDO name="c2" type="CMV_A"/>
			<SDO name="c3" type="CMV_A"/>
			<DA name="seqT" fc="MX" bType="Enum" type="SequenceKind">
				<Val>pos-neg-zero</Val>
			</DA>
		</DOType>
		<DOType id="DPC_SBOE" cdc="DPC">
			<DA name="stVal" fc="ST" bType="Dbpos" dchg="true"/>
			<DA name="q" fc="ST" bType="Quality" qchg="true"/>
			<DA name="t" fc="ST" bType="Timestamp"/>
			<DA name="ctlModel" fc="CF" bType="Enum" type="CtlModelKind">
				<Val>sbo-with-enhanced-security</Val>
			</DA>
			<DA name="sboTimeout" fc="CF" bType="INT32U">
				<Val>30000</Val>
			</DA>
			<DA name="SBOw" fc="CO" bType="Struct" type="DPCOperate"/>
			<DA name="Oper" fc="CO" bType="Struct" type="DPCOperate"/>
			<DA name="Cancel" fc="CO" bType="Struct" type="DPCCancel"/>
		</DOType>
		<DOType id="DPC_ST" cdc="DPC">
			<DA name="stVal" fc="ST" bType="Dbpos" dchg="true"/>
			<DA name="q" fc="ST" bType="Quality" qchg="true"/>
			<DA name="t" fc="ST" bType="Timestamp"/>
			<DA name="ctlModel" fc="CF" bType="Enum" type="CtlModelKind">
				<Val>status-only</Val>
			</DA>
		</DOType>
		<DOType id="SPC_ST" cdc="SPC">
			<DA name="stVal" fc="ST" bType="BOOLEAN" dchg="true"/>
			<DA name="q" fc="ST" bType="Quality" qchg="true"/>
			<DA name="t" fc="ST" bType="Timestamp"/>
			<DA name="ctlModel" fc="CF" bType="Enum" type="CtlModelKind">
				<Val>status-only</Val>
			</DA>
		</DOType>
		<DOType id="SPC_DIR" cdc="SPC">
			<DA name="stVal" fc="ST" bType="BOOLEAN" dchg="true"/>
			<DA name="q" fc="ST" bType="Quality" qchg="true"/>
			<DA name="t" fc="ST" bType="Timestamp"/>
			<DA name="ctlModel" fc="CF" bType="Enum" type="CtlModelKind">
				<Val>direct-with-normal-security</Val>
			</DA>
			<DA name="Oper" fc="CO" bType="Struct" type="SPCOperate"/>
		</DOType>
		<DOType id="ISC_A" cdc="ISC">
			<DA name="valWTr" fc="ST" bType="Struct" type="ValWithTrans" dchg="true"/>
			<DA name="q" fc="ST" bType="Quality" qchg="true"/>
			<DA name="t" fc="ST" bType="Timestamp"/>
			<DA name="ctlModel" fc="CF" bType="Enum" type="CtlModelKind">
				<Val>status-only</Val>
			</DA>
		</DOType>
		<DOType id="ASG_SG" cdc="ASG">
			<DA name="setMag" fc="SG" bType="Struct" type="AnalogueValue"/>
			<DA name="minVal" fc="CF" bType="Struct" type="AnalogueValue"/>
			<DA name="maxVal" fc="CF" bType="Struct" type="AnalogueValue"/>
			<DA name="stepSize" fc="CF" bType="Struct" type="AnalogueValue"/>
		</DOType>
		<DOType id="ASG_SP" cdc="ASG">
			<DA name="setMag" fc="SP" bType="Struct" type="AnalogueValue"/>
		</DOType>
		<DOType id="ING_SG" cdc="ING">
			<DA name="setVal" fc="SG" bType="INT32"/>
			<DA name="minVal" fc="CF" bType="INT32"/>
			<DA name="maxVal" fc="CF" bType="INT32"/>
			<DA name="stepSize" fc="CF" bType="INT32U"/>
		</DOType>
		<DOType id="SAV_A" cdc="SAV">
			<DA name="instMag" fc="MX" bType="Struct" type="AnalogueValue_I"/>
			<DA name="q" fc="MX" bType="Quality" qchg="true"/>
		</DOType>
		<DAType id="AnalogueValue">
			<BDA name="f" bType="FLOAT32"/>
		</DAType>
		<DAType id="AnalogueValue_I">
			<BDA name="i" bType="INT32"/>
		</DAType>
		<DAType id="Vector">
			<BDA name="mag" bType="Struct" type="AnalogueValue"/>
			<BDA name="ang" bType="Struct" type="AnalogueValue"/>
		</DAType>
		<DAType id="Originator">
			<BDA name="orCat" bType="Enum" type="OriginatorCategoryKind"/>
			<BDA name="orIdent" bType="Octet64"/>
		</DAType>
		<DAType id="DPCOperate">
			<BDA name="ctlVal" bType="BOOLEAN"/>
			<BDA name="origin" bType="Struct" type="Originator"/>
			<BDA name="ctlNum" bType="INT8U"/>
			<BDA name="T" bType="Timestamp"/>
			<BDA name="Test" bType="BOOLEAN"/>
			<BDA name="Check" bType="Check"/>
		</DAType>
		<DAType id="DPCCancel">
			<BDA name="ctlVal" bType="BOOLEAN"/>
			<BDA name="origin" bType="Struct" type="Originator"/>
			<BDA name="ctlNum" bType="INT8U"/>
			<BDA name="T" bType="Timestamp"/>
			<BDA name="Test" bType="BOOLEAN"/>
		</DAType>
		<DAType id="SPCOperate">
			<BDA name="ctlVal" bType="BOOLEAN"/>
			<BDA name="origin" bType="Struct" type="Originator"/>
			<BDA name="ctlNum" bType="INT8U"/>
			<BDA name="T" bType="Timestamp"/>
			<BDA name="Test" bType="BOOLEAN"/>
			<BDA name="Check" bType="Check"/>
		</DAType>
		<DAType id="ValWithTrans">
			<BDA name="posVal" bType="INT8"/>
			<BDA name="transInd" bType="BOOLEAN"/>
		</DAType>
		<EnumType id="BehaviourModeKind">
			<EnumVal ord="1">on</EnumVal>
			<EnumVal ord="2">on-blocked</EnumVal>
			<EnumVal ord="3">test</EnumVal>
			<EnumVal ord="4">test/blocked</EnumVal>
			<EnumVal ord="5">off</EnumVal>
		</EnumType>
		<EnumType id="HealthKind">
			<EnumVal ord="1">Ok</EnumVal>
			<EnumVal ord="2">Warning</EnumVal>
			<EnumVal ord="3">Alarm</EnumVal>
		</EnumType>
		<EnumType id="CtlModelKind">
			<EnumVal ord="0">status-only</EnumVal>
			<EnumVal ord="1">direct-with-normal-security</EnumVal>
			<EnumVal ord="2">sbo-with-normal-security</EnumVal>
			<EnumVal ord="3">direct-with-enhanced-security</EnumVal>
			<EnumVal ord="4">sbo-with-enhanced-security</EnumVal>
		</EnumType>
		<EnumType id="OriginatorCategoryKind">
			<EnumVal ord="0">not-supported</EnumVal>
			<EnumVal ord="1">bay-control</EnumVal>
			<EnumVal ord="2">station-control</EnumVal>
			<EnumVal ord="3">remote-control</EnumVal>
			<EnumVal ord="4">automatic-bay</EnumVal>
			<EnumVal ord="5">automatic-station</EnumVal>
			<EnumVal ord="6">automatic-remote</EnumVal>
			<EnumVal ord="7">maintenance</EnumVal>
			<EnumVal ord="8">process</EnumVal>
		</EnumType>
		<EnumType id="FaultDirectionKind">
			<EnumVal ord="0">unknown</EnumVal>
			<EnumVal ord="1">forward</EnumVal>
			<EnumVal ord="2">backward</EnumVal>
			<EnumVal ord="3">both</EnumVal>
		</EnumType>
		<EnumType id="PhaseFaultDirectionKind">
			<EnumVal ord="0">unknown</EnumVal>
			<EnumVal ord="1">forward</EnumVal>
			<EnumVal ord="2">backward</EnumVal>
		</EnumType>
		<EnumType id="SwitchFunctionKind">
			<EnumVal ord="1">Load Break Switch</EnumVal>
			<EnumVal ord="2">Disconnector</EnumVal>
			<EnumVal ord="3">Earthing Switch</EnumVal>
			<EnumVal ord="4">High Speed Earthing Switch</EnumVal>
		</EnumType>
		<EnumType id="SequenceKind">
			<EnumVal ord="0">pos-neg-zero</EnumVal>
			<EnumVal ord="1">dir-quad-zero</EnumVal>
		</EnumType>
	</DataTypeTemplates>
</SCL>
