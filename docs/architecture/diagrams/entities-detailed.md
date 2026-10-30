# Diagrama de Entidades e Relacionamentos (UML Detalhado)

Este diagrama representa a estrutura exata do backend em Go (models dos microsserviços), refletindo as propriedades e os métodos conforme definidos no código-fonte.

```mermaid
classDiagram
    direction LR
    %% Relacionamentos
    Account "1" -- "*" Lead : TenantID
    Account "1" -- "*" Property : TenantID
    Account "1" -- "*" Broker : TenantID
    Account "1" -- "*" Deal : TenantID
    Account "1" -- "*" Contract : TenantID
    Account "1" -- "*" Transaction : TenantID
    
    Broker "1" -- "*" Lead : AssignedBrokerID
    Broker "1..*" -- "*" Property : CapturingBrokers
    Broker "1..*" -- "*" Deal : BrokerID / BrokerParticipations
    
    Lead "1..*" -- "*" Property : PropertyIDs / Owners / InterestedLeads
    Lead "1" -- "1" Contract : ContractID
    
    Property "1" -- "*" Deal : PropertyID
    Lead "1..*" -- "*" Deal : OwnerID / CustomerID / LeadID
    
    Deal "1" -- "1" Contract : DealID
    Property "1" -- "*" Contract : PropertyID
    
    Contract "1" -- "*" Transaction : ContractID

    %% Classes e Entidades (Refletindo Structs Exatos do Go)
    
    class Account {
        +String ID
        +AccountType Type
        +User Individual
        +Company Company
        +Time CreatedAt
        +Time UpdatedAt
        +Validate() error
    }

    class Lead {
        +String ID
        +String DisplayID
        +String Name
        +String Email
        +String Phone
        +String Avatar
        +LeadSource Source
        +LeadStatus Status
        +Time CreatedAt
        +Time UpdatedAt
        +Time LastInteraction
        +String TenantID
        +AuthMetadata AuthMetadata
        +LeadProperty PropertyInterest
        +[]String PropertyIDs
        +String AssignedBrokerID
        +[]LeadHistory History
        +Boolean IsClient
        +ClientRole Role
        +String CaptacaoPropertyID
        +String OwnerLeadID
        +String ContractID
        +Map SLARetries
        +Validate() error
        +AddHistory(action, userID, details, oldStatus, newStatus)
        +ConvertToClient(role, contractID)
    }

    class Broker {
        +String ID
        +String Name
        +String Role
        +String Email
        +String Avatar
        +Boolean Active
        +Int MaxActiveLeads
        +String TenantID
        +Time CreatedAt
        +Time UpdatedAt
    }

    class Property {
        +String ID
        +String DisplayID
        +String ExternalID
        +String Title
        +String Description
        +PropertyStatus Status
        +PublicationStatus PublicationStatus
        +PropertyType PropertyType
        +ListingType ListingType
        +PropertyCondition Condition
        +Address Address
        +Pricing Pricing
        +Areas Areas
        +Features Features
        +[]String Amenities
        +Media Media
        +Legal Legal
        +[]OwnerRef Owners
        +[]CapturingBrokerRef CapturingBrokers
        +[]ListingType AvailableFor
        +String IntermediationContractID
        +[]InterestedLeadRef InterestedLeads
        +String ActiveDealID
        +Boolean PartnershipPortfolio
        +[]String Tags
        +String TenantID
        +Validate() error
    }

    class Deal {
        +String ID
        +String DisplayID
        +Type Type
        +Status Status
        +String PropertyID
        +String OwnerID
        +String CustomerID
        +String LeadID
        +String BrokerID
        +Float64 Value
        +Float64 Commission
        +Time CreatedAt
        +Time UpdatedAt
        +String TenantID
        +[]BrokerParticipation BrokerParticipations
        +[]Document Documents
        +[]History History
        +[]Proposal Proposals
        +DueDiligence DueDiligence
        +[]Signer Signers
        +[]ChecklistItem Checklist
        +AddBrokerParticipation(brokerID, role, commission, percentage)
        +AddDocument(docType, description)
        +UpdateDocument(docID, url, userID)
        +AddHistory(status, userID, notes)
        +AddProposal(value, paymentMethod, installments, downPayment, createdBy, notes)
        +UpdateDueDiligence(score, cndStatus, legalOpinion, status)
        +AddSigner(name, email, role)
        +UpdateSignerStatus(signerID, status, signatureURL)
        +InitChecklist(items)
        +AddChecklistItem(name, category, required, description)
        +RemoveChecklistItem(itemID)
        +UpdateChecklistItemStatus(itemID, status, rejectionReason)
        +SolicitChecklistItem(itemID)
    }

    class Contract {
        +String ID
        +String DisplayID
        +String DealID
        +Status Status
        +String Content
        +[]Signatory Signatories
        +Time StartDate
        +Time EndDate
        +ContractType Type
        +String PropertyID
        +String PropertyTitle
        +String Client
        +Float64 Value
        +Float64 CommissionPct
        +String AdjustIndex
        +Boolean AutoAdjust
        +String NextAdjustment
        +Time CreatedAt
        +Time UpdatedAt
        +String TenantID
    }

    class Transaction {
        +String ID
        +String ContractID
        +Type Type
        +String Description
        +Float64 Value
        +Time Date
        +Status Status
        +Time DueDate
        +Time PaymentDate
        +String PaymentMethod
        +String Category
        +String Month
        +Float64 GrossValue
        +Float64 AdminFee
        +Float64 MaintenanceDeductions
        +Float64 UtilityDeductions
        +String TenantID
        +Time CreatedAt
        +Time UpdatedAt
    }
```
